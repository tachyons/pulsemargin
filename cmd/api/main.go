package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/exasol/exasol-driver-go"
)

type CartItem struct {
	SKU      string  `json:"sku"`
	Category string  `json:"category"`
	Size     string  `json:"size"`
	Price    float64 `json:"price"`
}

type CheckoutPayload struct {
	CheckoutID  string     `json:"checkout_id"`
	CustomerID  string     `json:"customer_id"`
	Pincode     string     `json:"pincode"`
	PaymentMode string     `json:"payment_mode"` // "COD" or "PREPAID"
	Items       []CartItem `json:"items"`
}

type InterceptionDecision struct {
	CheckoutID    string   `json:"checkout_id"`
	RiskScore     float64  `json:"risk_score"`
	ActionTier    string   `json:"action_tier"`
	Interventions []string `json:"recommended_interventions"`
	LatencyMs     int64    `json:"database_latency_ms"`
}

type Server struct {
	db *sql.DB
}

func main() {
	host := getEnv("EXASOL_HOST", "localhost")
	port := getEnv("EXASOL_PORT", "8563")
	user := getEnv("EXASOL_USER", "sys")
	pass := getEnv("EXASOL_PASS", "exasol")
	schema := getEnv("EXASOL_SCHEMA", "PULSE_MARGIN")
	apiPort := getEnv("PORT", "8080")

	// Exasol Go Driver DSN format
	dsn := fmt.Sprintf("exa:%s:%s;user=%s;password=%s;schema=%s", host, port, user, pass, schema)

	log.Printf("Connecting to Exasol instance at %s:%s (schema: %s)...", host, port, schema)
	db, err := sql.Open("exasol", dsn)
	if err != nil {
		log.Fatalf("Failed to initialize Exasol connection: %v", err)
	}
	defer db.Close()

	// High concurrency pool tuned for in-memory analytics
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(15 * time.Minute)

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()

	if err := db.PingContext(pingCtx); err != nil {
		log.Printf("WARNING: Exasol ping check failed: %v. Running in detached mode.", err)
	} else {
		log.Println("Connection to Exasol established successfully.")
	}

	srv := &Server{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", srv.handleHealth)
	mux.HandleFunc("/api/v1/checkout/score", srv.handleCheckoutScore)

	httpServer := &http.Server{
		Addr:         ":" + apiPort,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("PulseMargin API serving on port %s", apiPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down API server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server gracefully stopped.")
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"UP"}`))
}

func (s *Server) handleCheckoutScore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var payload CheckoutPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"Malformed JSON payload"}`, http.StatusBadRequest)
		return
	}

	if payload.CheckoutID == "" || payload.CustomerID == "" || len(payload.Items) == 0 {
		http.Error(w, `{"error":"Missing required fields (checkout_id, customer_id, items)"}`, http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
	defer cancel()

	start := time.Now()

	// 1. Detect cart bracketing (buying multiple sizes of the same category)
	categorySizeMap := make(map[string]map[string]bool)
	hasBracketing := false
	var cartTotal float64

	for _, it := range payload.Items {
		cartTotal += it.Price
		if categorySizeMap[it.Category] == nil {
			categorySizeMap[it.Category] = make(map[string]bool)
		}
		categorySizeMap[it.Category][it.Size] = true
		if len(categorySizeMap[it.Category]) > 1 {
			hasBracketing = true
		}
	}

	isCOD := strings.EqualFold(payload.PaymentMode, "COD")

	// 2. Build parameterized placeholders for SKUs
	skuPlaceholders := make([]string, len(payload.Items))
	skuArgs := make([]interface{}, len(payload.Items))
	for i, item := range payload.Items {
		skuPlaceholders[i] = "?"
		skuArgs[i] = item.SKU
	}

	// 3. Analytical query aggregating profile & telemetry, evaluated via Python UDF in-database
	query := fmt.Sprintf(`
		WITH cart_skus AS (
			SELECT COALESCE(MAX(size_skew_factor), 0.0) AS max_skew
			FROM product_return_telemetry
			WHERE sku IN (%s)
		),
		cust_stats AS (
			SELECT
				CASE WHEN total_orders > 0 THEN CAST(total_returns AS DOUBLE) / total_orders ELSE 0.15 END AS cust_ret_rate
			FROM customer_profiles
			WHERE customer_id = ?
		),
		pin_stats AS (
			SELECT COALESCE(rto_rate, 0.10) AS pin_ret_rate
			FROM pincode_risk_factors
			WHERE pincode = ?
		)
		SELECT
			predict_return_risk(
				(SELECT cust_ret_rate FROM cust_stats),
				(SELECT pin_ret_rate FROM pin_stats),
				?,
				(SELECT max_skew FROM cart_skus),
				?,
				?
			) AS propensity_score
		FROM cart_skus;
	`, strings.Join(skuPlaceholders, ","))

	var queryArgs []interface{}
	queryArgs = append(queryArgs, skuArgs...)
	queryArgs = append(queryArgs, payload.CustomerID, payload.Pincode, hasBracketing, cartTotal, isCOD)

	var score sql.NullFloat64
	err := s.db.QueryRowContext(ctx, query, queryArgs...).Scan(&score)
	if err != nil {
		log.Printf("Query error on Exasol: %v", err)
		http.Error(w, fmt.Sprintf(`{"error":"Exasol query failed: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	riskScore := 0.20
	if score.Valid {
		riskScore = score.Float64
	}

	// 4. Evaluate Dynamic Interception Actions
	decision := InterceptionDecision{
		CheckoutID:    payload.CheckoutID,
		RiskScore:     riskScore,
		LatencyMs:     time.Since(start).Milliseconds(),
		Interventions: make([]string, 0),
	}

	switch {
	case riskScore >= 0.75:
		decision.ActionTier = "HIGH_RISK"
		decision.Interventions = append(decision.Interventions,
			"Trigger conversational WhatsApp sizing verification prior to dispatch",
			"Restrict COD: Offer ₹100 instant prepaid discount incentive",
			"Route order to surface economy tier to reduce return airfreight margin drag",
		)
	case riskScore >= 0.45:
		decision.ActionTier = "MODERATE_RISK"
		decision.Interventions = append(decision.Interventions,
			"Send automated sizing chart re-check notification via SMS",
			"Offer 5% non-refundable wallet cashback upon delivery confirmation",
		)
	default:
		decision.ActionTier = "LOW_RISK"
		decision.Interventions = append(decision.Interventions,
			"Fast-Track fulfillment with standard courier partner",
		)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(decision)
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
