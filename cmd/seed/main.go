package main

import (
	"context"
	"flag"
	"log"
	"time"

	"pulsemargin/internal/config"
	"pulsemargin/internal/database"
	"pulsemargin/internal/seed"
)

func main() {
	csvPath := flag.String("csv", "data/customer_profiles.csv", "customer profiles CSV to import")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	db, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatalf("Connection failed: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	log.Println("Starting Exasol synthetic data pipeline...")

	if err := seed.Run(ctx, db, *csvPath); err != nil {
		log.Fatalf("Data seeding failed: %v", err)
	}

	log.Println("Data seeding completed successfully.")
}
