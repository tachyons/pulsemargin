package database

import (
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

// Open creates the Exasol connection pool. No network round trip happens until
// the pool is first used, so callers decide whether to Ping.
func Open(cfg Config) (*sql.DB, error) {
	// Standard Exasol Go connection URI. Local/dev instances ship a
	// self-signed certificate, so server certificate validation is disabled.
	dsn := fmt.Sprintf("exa:%s:%d;user=%s;password=%s;schema=%s;validateservercertificate=0",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Schema,
	)

	db, err := sql.Open("exasol", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open exasol connection: %w", err)
	}

	// In-memory analytical pools perform best with persistent connections
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(15 * time.Minute)

	return db, nil
}
