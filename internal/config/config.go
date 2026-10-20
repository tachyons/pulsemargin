package config

import (
	"fmt"
	"os"
	"strconv"

	"pulsemargin/internal/database"
)

// Config is the process configuration shared by the API and seeder commands.
type Config struct {
	Database database.Config
	APIPort  string
}

// Load reads configuration from the environment, falling back to local defaults.
func Load() (Config, error) {
	port, err := strconv.Atoi(getEnv("EXASOL_PORT", "8563"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid EXASOL_PORT: %w", err)
	}

	return Config{
		Database: database.Config{
			Host:     getEnv("EXASOL_HOST", "localhost"),
			Port:     port,
			User:     getEnv("EXASOL_USER", "sys"),
			Password: getEnv("EXASOL_PASS", "exasol"),
			Schema:   getEnv("EXASOL_SCHEMA", "PULSE_MARGIN"),
		},
		APIPort: getEnv("PORT", "8080"),
	}, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
