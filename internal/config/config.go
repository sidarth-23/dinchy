// Package config loads application startup configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
)

// Config holds all startup configuration values for the Dinchy server.
type Config struct {
	// Addr is the public listen address for the main HTTP server.
	Addr string `env:"DINCHY_ADDR" validate:"required"`
	// InternalAddr is the listen address for internal health/ready endpoints.
	InternalAddr string `env:"DINCHY_INTERNAL_ADDR" validate:"required"`
	// DBPath is the file path for the SQLite database.
	DBPath string `env:"DINCHY_DB_PATH" validate:"required"`
	// DevMode enables development mode (relaxed CSP, frontend proxy).
	DevMode bool `env:"DINCHY_DEV" validate:"-"`
	// DevProxyURL is the Vite dev server URL to proxy frontend requests to in dev mode.
	DevProxyURL string `env:"DINCHY_DEV_PROXY_URL" validate:"omitempty,url"`
	// RequireHTTPSForAuth enforces HTTPS on all auth endpoints when true.
	RequireHTTPSForAuth bool `env:"DINCHY_REQUIRE_HTTPS_FOR_AUTH" validate:"-"`
}

// Load reads configuration from an env file (if present) and environment variables,
// then validates the result. It returns an error if the env file cannot be loaded
// or any required field fails validation.
func Load() (Config, error) {
	if err := loadEnvFile(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}

	cfg := Config{
		Addr:                envOr("DINCHY_ADDR", ":8080"),
		InternalAddr:        envOr("DINCHY_INTERNAL_ADDR", ":9090"),
		DBPath:              envOr("DINCHY_DB_PATH", "./dinchy.db"),
		DevMode:             envBool("DINCHY_DEV"),
		DevProxyURL:         envOr("DINCHY_DEV_PROXY_URL", "http://127.0.0.1:5173"),
		RequireHTTPSForAuth: envBool("DINCHY_REQUIRE_HTTPS_FOR_AUTH"),
	}

	if err := validator.New().Struct(cfg); err != nil {
		return Config{}, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "1", "true", "t", "yes", "on":
		return true
	}
	return false
}
