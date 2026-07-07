// IMPORTANT: This file keeps a few startup-only diagnostic literals.
// They are internal failure details only and are never returned to users.
// Package config loads application startup configuration from environment variables.
package config

import (
	"reflect"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/validation"
)

// Config holds all startup configuration values for the Dinchy server.
type Config struct {
	// Addr is the public listen address for the main HTTP server.
	Addr string `env:"DINCHY_ADDR" validate:"required"`
	// InternalAddr is the listen address for internal health/ready endpoints.
	InternalAddr string `env:"DINCHY_INTERNAL_ADDR" validate:"required"`
	// Database contains persistent storage backend settings.
	Database DatabaseConfig
	// DevMode enables development mode (relaxed CSP, frontend proxy).
	DevMode bool `env:"DINCHY_DEV"`
	// DevProxyURL is the Vite dev server URL to proxy frontend requests to in dev mode.
	// Required when DevMode is true.
	DevProxyURL string `env:"DINCHY_DEV_PROXY_URL" validate:"required_if=DevMode true,omitempty,http_url"`
	// RequireHTTPSForAuth enforces HTTPS on all auth endpoints when true.
	RequireHTTPSForAuth bool `env:"DINCHY_REQUIRE_HTTPS_FOR_AUTH"`
	// PublicBaseURL is the externally reachable base URL used to build links in
	// outbound email (invitation and password reset). Required when SMTP is enabled.
	PublicBaseURL string `env:"DINCHY_PUBLIC_BASE_URL" validate:"omitempty,http_url"`
	// Auth contains authentication behavior and lifetime settings.
	Auth AuthConfig
	// SSO contains startup SSO provider values loaded from environment.
	SSO SSOEnvConfig
	// SMTP contains outbound email settings for password reset and invitation flows.
	SMTP SMTPConfig
	// Cache contains optional cache store settings for ephemeral state.
	Cache CacheConfig
	// EventBus contains the Redis stream settings for durable in-app events.
	EventBus EventBusConfig
	// Logging controls application log formatting and level.
	Logging LoggingConfig
	// Telemetry controls OpenTelemetry logs and traces.
	Telemetry TelemetryConfig
	// SSOProviders contains the enabled SSO providers derived from env configuration.
	SSOProviders []SSOProviderConfig
}

// Load reads configuration from an env file (if present) and environment variables,
// then validates the result. It returns an error if the env file cannot be loaded
// or any required field fails validation.
func Load() (Config, error) {
	if err := loadEnvFile(); err != nil {
		return Config{}, apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed), apperrors.WithCause(err))
	}

	cfg := Config{
		Addr:         ":8080",
		InternalAddr: ":9090",
		DevProxyURL:  "http://127.0.0.1:5173",
		Auth:         DefaultAuth(),
		SMTP:         DefaultSMTP(),
		Cache:        DefaultCache(),
		EventBus:     DefaultEventBus(),
		Logging:      DefaultLogging(),
		Telemetry:    DefaultTelemetry(),
	}
	if err := loadFromEnvValue(reflect.ValueOf(&cfg).Elem()); err != nil {
		return Config{}, err
	}
	if err := applyModsValue(reflect.ValueOf(&cfg).Elem()); err != nil {
		return Config{}, err
	}
	cfg.SSOProviders = configuredSSOProviders(cfg)

	v := validation.New()
	if err := v.Struct(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}
