// Package config loads application startup configuration from environment variables.
package config

import (
	"os"
	"strconv"
)

// Config holds all startup configuration values for the Dinchy server.
type Config struct {
	Addr               string
	DBPath             string
	DevMode            bool
	DevProxyURL        string
	RequireHTTPSForAuth bool
}

// FromEnv reads configuration from environment variables with sensible defaults.
func FromEnv() Config {
	devMode := os.Getenv("DINCHY_DEV") == "1"
	requireHTTPS := os.Getenv("DINCHY_REQUIRE_HTTPS_FOR_AUTH") == "1"
	addr := envOr("DINCHY_ADDR", ":8080")
	dbPath := envOr("DINCHY_DB_PATH", "./dinchy.db")
	devProxyURL := envOr("DINCHY_DEV_PROXY_URL", "http://127.0.0.1:5173")
	if v := os.Getenv("DINCHY_DEV_BOOL"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			devMode = parsed
		}
	}
	return Config{Addr: addr, DBPath: dbPath, DevMode: devMode, DevProxyURL: devProxyURL, RequireHTTPSForAuth: requireHTTPS}
}

func envOr(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}
