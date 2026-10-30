package checkout

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type CartItem struct {
	SKU      string  `json:"sku"`
	Category string  `json:"category"`
	Size     string  `json:"size"`
	Price    float64 `json:"price"`
}

type Payload struct {
	CheckoutID  string     `json:"checkout_id"`
	CustomerID  string     `json:"customer_id"`
	Pincode     string     `json:"pincode"`
	PaymentMode string     `json:"payment_mode"` // "COD" or "PREPAID"
	Items       []CartItem `json:"items"`
}

type Decision struct {
	CheckoutID    string   `json:"checkout_id"`
	RiskScore     float64  `json:"risk_score"`
	ActionTier    string   `json:"action_tier"`
	Interventions []string `json:"recommended_interventions"`
	LatencyMs     int64    `json:"database_latency_ms"`
	ModelVersion  string   `json:"model_version,omitempty"`
}

// DatabaseScorer evaluates checkout return risk inside Exasol using the predict_return_risk UDF.
type DatabaseScorer struct {
	db *sql.DB
}

// NewDatabaseScorer constructs a database-backed checkout scorer.
func NewDatabaseScorer(db *sql.DB) *DatabaseScorer {
	return &DatabaseScorer{db: db}
}

// NewScorer creates a database-backed Scorer for backward compatibility.
func NewScorer(db *sql.DB) Scorer {
	return NewDatabaseScorer(db)
}

// Score runs the in-database risk model and maps the returned propensity to an
// interception tier.
func (s *DatabaseScorer) Score(ctx context.Context, payload Payload) (Decision, error) {
	start := time.Now()

	cartTotal, hasBracketing := analyzeCart(payload.Items)
	query, args := scoreQuery(payload, cartTotal, hasBracketing)

	var score sql.NullFloat64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&score); err != nil {
		return Decision{}, fmt.Errorf("score checkout %s: %w", payload.CheckoutID, err)
	}

	riskScore := 0.25
	if score.Valid {
		riskScore = score.Float64
	}

	decision := newDecision(payload.CheckoutID, riskScore, time.Since(start).Milliseconds())
	decision.ModelVersion = "exasol_udf_v1"
	return decision, nil
}

// analyzeCart totals the cart and detects size bracketing: multiple sizes of
// the same category in one order.
func analyzeCart(items []CartItem) (total float64, hasBracketing bool) {
	sizesByCategory := make(map[string]map[string]bool)

	for _, item := range items {
		total += item.Price
		if sizesByCategory[item.Category] == nil {
			sizesByCategory[item.Category] = make(map[string]bool)
		}
		sizesByCategory[item.Category][item.Size] = true
		if len(sizesByCategory[item.Category]) > 1 {
			hasBracketing = true
		}
	}

	return total, hasBracketing
}

func scoreQuery(payload Payload, cartTotal float64, hasBracketing bool) (string, []any) {
	placeholders := make([]string, len(payload.Items))
	args := make([]any, 0, len(payload.Items)+5)
	for i, item := range payload.Items {
		placeholders[i] = "?"
		args = append(args, item.SKU)
	}
	// The Exasol driver reports untyped placeholders as VARCHAR, so booleans and
	// doubles are bound as strings and cast server-side to the types the UDF expects.
	args = append(args,
		payload.CustomerID,
		payload.Pincode,
		strconv.FormatBool(hasBracketing),
		strconv.FormatFloat(cartTotal, 'f', -1, 64),
		strconv.FormatBool(strings.EqualFold(payload.PaymentMode, "COD")),
	)

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
				CAST(? AS BOOLEAN),
				(SELECT max_skew FROM cart_skus),
				CAST(? AS DOUBLE PRECISION),
				CAST(? AS BOOLEAN)
			) AS propensity_score
		FROM cart_skus;
	`, strings.Join(placeholders, ","))

	return query, args
}

func newDecision(checkoutID string, riskScore float64, latencyMs int64) Decision {
	decision := Decision{
		CheckoutID:    checkoutID,
		RiskScore:     riskScore,
		LatencyMs:     latencyMs,
		Interventions: make([]string, 0),
	}

	switch {
	case riskScore >= 0.80:
		decision.ActionTier = "HIGH_RISK"
		decision.Interventions = append(decision.Interventions,
			"Trigger conversational WhatsApp sizing verification prior to fulfillment",
			"Restrict COD: Offer ₹100 instant discount for prepaid conversion",
			"Route order to surface courier tier to minimize air-express return loss",
		)
	case riskScore >= 0.50:
		decision.ActionTier = "MEDIUM_RISK"
		decision.Interventions = append(decision.Interventions,
			"Send automated sizing chart verification prompt via SMS/Email",
			"Offer 5% non-refundable store credit coupon at delivery confirmation",
		)
	default:
		decision.ActionTier = "LOW_RISK"
		decision.Interventions = append(decision.Interventions,
			"Standard Fast-Track Fulfillment",
		)
	}

	return decision
}
