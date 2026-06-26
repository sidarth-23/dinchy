// IMPORTANT: This file keeps a few startup-only diagnostic literals.
// They are internal failure details only and are never returned to users.
// Package config loads application startup configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// Config holds all startup configuration values for the Dinchy server.
type Config struct {
	// Addr is the public listen address for the main HTTP server.
	Addr string `env:"DINCHY_ADDR" validate:"required"`
	// InternalAddr is the listen address for internal health/ready endpoints.
	InternalAddr string `env:"DINCHY_INTERNAL_ADDR" validate:"required"`
	// DBBackend selects the database implementation to use.
	DBBackend string `env:"DINCHY_DB_BACKEND"`
	// DBPath is the file path for the SQLite database.
	DBPath string `env:"DINCHY_DB_PATH"`
	// PostgresDSN is the connection string for the PostgreSQL backend.
	PostgresDSN string `env:"DINCHY_POSTGRES_DSN"`
	// DevMode enables development mode (relaxed CSP, frontend proxy).
	DevMode bool `env:"DINCHY_DEV"`
	// DevProxyURL is the Vite dev server URL to proxy frontend requests to in dev mode.
	// Required when DevMode is true.
	DevProxyURL string `env:"DINCHY_DEV_PROXY_URL" validate:"required_if=DevMode true,omitempty,url"`
	// RequireHTTPSForAuth enforces HTTPS on all auth endpoints when true.
	RequireHTTPSForAuth bool `env:"DINCHY_REQUIRE_HTTPS_FOR_AUTH"`
}

// defaultConfig returns a Config pre-populated with sensible defaults.
func defaultConfig() Config {
	return Config{
		Addr:         ":8080",
		InternalAddr: ":9090",
		DBBackend:    "sqlite",
		DBPath:       "./dinchy.db",
		DevProxyURL:  "http://127.0.0.1:5173",
	}
}

// Load reads configuration from an env file (if present) and environment variables,
// then validates the result. It returns an error if the env file cannot be loaded
// or any required field fails validation.
func Load() (Config, error) {
	if err := loadEnvFile(); err != nil {
		return Config{}, apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed), apperrors.WithCause(err))
	}

	cfg := defaultConfig()
	if err := loadFromEnv(&cfg); err != nil {
		return Config{}, err
	}

	if err := validator.New().Struct(cfg); err != nil {
		return Config{}, apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(err))
	}

	switch cfg.DBBackend {
	case "", "sqlite":
		if cfg.DBPath == "" {
			return Config{}, apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_DB_PATH is required for sqlite backend")))
		}
	case "postgres":
		if cfg.PostgresDSN == "" {
			return Config{}, apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_POSTGRES_DSN is required for postgres backend")))
		}
	default:
		return Config{}, apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("unsupported database backend %q", cfg.DBBackend)))
	}

	return cfg, nil
}

// loadFromEnv iterates Config fields, reads the env tag to find the env var name,
// and overrides the field value only when the env var is non-empty.
func loadFromEnv(cfg *Config) error {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		envKey := field.Tag.Get("env")
		if envKey == "" {
			continue
		}
		raw := os.Getenv(envKey)
		if raw == "" {
			continue // keep the default
		}
		switch field.Type.Kind() {
		case reflect.String:
			v.Field(i).SetString(raw)
		case reflect.Bool:
			v.Field(i).SetBool(parseBool(raw))
		default:
			return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
				apperrors.WithCause(fmt.Errorf("unsupported env field type %q for %q", field.Type.Kind().String(), field.Name)),
				apperrors.WithField(apperrors.FieldName(field.Name)),
				apperrors.WithKind(apperrors.FieldKindOf(field.Type.Kind())),
			)
		}
	}
	return nil
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "t", "yes", "on":
		return true
	}
	return false
}
