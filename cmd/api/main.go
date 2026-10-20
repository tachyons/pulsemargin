package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pulsemargin/internal/api"
	"pulsemargin/internal/checkout"
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

	if err := db.PingContext(pingCtx); err != nil {
		log.Printf("WARNING: Exasol ping check failed: %v. Running in detached mode.", err)
	} else {
		log.Println("Connection to Exasol established successfully.")
	}

	httpServer := &http.Server{
		Addr:         ":" + cfg.APIPort,
		Handler:      api.NewHandler(checkout.NewScorer(db)),
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
