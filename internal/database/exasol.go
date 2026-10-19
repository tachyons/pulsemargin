package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/exasol/exasol-driver-go"
)

type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Schema   string
}

func Connect(cfg Config) (*sql.DB, error) {
	// Standard Exasol Go connection URI
	dsn := fmt.Sprintf("exa:%s:%d;user=%s;password=%s;schema=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Schema,
	)

	db, err := sql.Open("exasol", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open exasol connection: %w", err)
	}

	// In-memory analytical pools perform best with persistent connections
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping exasol instance: %w", err)
	}

	return db, nil
}
