package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pulsemargin/internal/api"
	"pulsemargin/internal/checkout"
	"pulsemargin/internal/checkout/models"
	"pulsemargin/internal/config"
	"pulsemargin/internal/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Connecting to Exasol instance at %s:%d (schema: %s)...",
		cfg.Database.Host, cfg.Database.Port, cfg.Database.Schema)

	db, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize Exasol connection: %v", err)
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dbConnected := true
	if err := db.PingContext(pingCtx); err != nil {
		dbConnected = false
		log.Printf("WARNING: Exasol ping check failed: %v. Running in detached mode.", err)
	} else {
		log.Println("Connection to Exasol established successfully.")
	}

	var scorer checkout.Scorer
	switch cfg.ScorerBackend {
	case "ml":
		log.Printf("Configured ML Scorer backend (model: %s)", cfg.MLModel)
		scorer = buildMLScorer(cfg, db, dbConnected)
	case "db":
		log.Println("Configured Database Scorer backend")
		scorer = checkout.NewDatabaseScorer(db)
	default: // "auto"
		if dbConnected {
			log.Println("Auto-selected Exasol Database Scorer (database connection active)")
			scorer = checkout.NewDatabaseScorer(db)
		} else {
			log.Printf("Exasol unavailable. Auto-fallback to ML Scorer (model: %s) with in-memory feature store", cfg.MLModel)
			scorer = buildMLScorer(cfg, db, false)
		}
	}

	httpServer := &http.Server{
		Addr:         ":" + cfg.APIPort,
		Handler:      api.NewHandler(scorer),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("PulseMargin API serving on port %s", cfg.APIPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-stopCtx.Done()

	log.Println("Shutting down API server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server gracefully stopped.")
}

func buildMLScorer(cfg config.Config, db *sql.DB, useDBStore bool) checkout.Scorer {
	var store checkout.FeatureStore
	if useDBStore && db != nil {
		log.Println("ML Scorer: using DatabaseFeatureStore")
		store = checkout.NewDatabaseFeatureStore(db)
	} else {
		log.Println("ML Scorer: using InMemoryFeatureStore")
		store = checkout.NewInMemoryFeatureStore()
	}

	var model checkout.MLModel
	switch cfg.MLModel {
	case "tree":
		model = models.NewDecisionTreeModel("tree_ensemble_v2.0")
	case "remote":
		fallback := models.NewLogisticRegressionModel("logistic_fallback_v1.0", models.DefaultLogisticWeights())
		model = models.NewRemoteModelClient(cfg.MLRemoteURL, 500*time.Millisecond, fallback)
	default: // "logistic"
		model = models.NewLogisticRegressionModel("logistic_regression_v1.2", models.DefaultLogisticWeights())
	}

	extractor := checkout.NewFeatureExtractor(store)
	categorizer := checkout.NewMLCategorizer()
	return checkout.NewMLScorer(extractor, model, categorizer)
}
