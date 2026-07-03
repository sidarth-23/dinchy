// IMPORTANT: This file keeps a few startup-only diagnostic literals.
// They are internal failure details only and are never returned to users.
// Package config loads application startup configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/validation"
)

type SMTPConfig struct {
	// Host is the SMTP server hostname used for outbound application email.
	Host string `env:"DINCHY_SMTP_HOST"`
	// Port is the SMTP server port; defaults to 587 when SMTP is enabled and no port is set.
	Port string `env:"DINCHY_SMTP_PORT"`
	// Username is the optional SMTP username for authenticated mail servers.
	Username string `env:"DINCHY_SMTP_USERNAME"`
	// Password is the optional SMTP password for authenticated mail servers.
	Password string `env:"DINCHY_SMTP_PASSWORD"`
	// From is the sender address used for password reset and invite emails.
	From string `env:"DINCHY_SMTP_FROM"`
}

type CacheConfig struct {
	// Backend selects the cache implementation. Empty disables the cache.
	Backend string `env:"DINCHY_CACHE_BACKEND"`
	// Addr is the network address for the configured cache backend.
	Addr string `env:"DINCHY_CACHE_ADDR"`
	// Username is the optional cache username.
	Username string `env:"DINCHY_CACHE_USERNAME"`
	// Password is the optional cache password.
	Password string `env:"DINCHY_CACHE_PASSWORD"`
	// Database selects the backend database or namespace when supported.
	Database int `env:"DINCHY_CACHE_DATABASE"`
	// KeyPrefix scopes all cache keys for this Dinchy instance.
	KeyPrefix string `env:"DINCHY_CACHE_KEY_PREFIX"`
}

type AuthConfig struct {
	// SessionCookieName is the HTTP cookie name used for Dinchy session tokens.
	SessionCookieName string `env:"DINCHY_AUTH_SESSION_COOKIE_NAME" validate:"required"`
	// SSOStateCookieName is the temporary HTTP cookie name used during SSO redirects.
	SSOStateCookieName string `env:"DINCHY_AUTH_SSO_STATE_COOKIE_NAME" validate:"required"`
	// SessionIdleTimeout is the maximum idle time before a session is considered expired.
	SessionIdleTimeout time.Duration `env:"DINCHY_AUTH_SESSION_IDLE_TIMEOUT"`
	// SessionMaxLifetime is the absolute maximum age of a session regardless of activity.
	SessionMaxLifetime time.Duration `env:"DINCHY_AUTH_SESSION_MAX_LIFETIME"`
	// SSOStateLifetime is the maximum age of a pending SSO redirect transaction.
	SSOStateLifetime time.Duration `env:"DINCHY_AUTH_SSO_STATE_LIFETIME"`
	// PasswordResetLifetime is the validity window for password reset tokens.
	PasswordResetLifetime time.Duration `env:"DINCHY_AUTH_PASSWORD_RESET_LIFETIME"`
	// TOTPIssuer is the issuer label shown in authenticator apps for Dinchy TOTP secrets.
	TOTPIssuer string `env:"DINCHY_AUTH_TOTP_ISSUER" validate:"required"`
	// DefaultOrganisationName is the organisation name created during first-user setup.
	DefaultOrganisationName string `env:"DINCHY_AUTH_DEFAULT_ORGANISATION_NAME" validate:"required"`
	// DefaultOrganisationSlug is the organisation slug created during first-user setup.
	DefaultOrganisationSlug string `env:"DINCHY_AUTH_DEFAULT_ORGANISATION_SLUG" validate:"required"`
}

func (c SMTPConfig) Enabled() bool {
	return strings.TrimSpace(c.Host) != "" || strings.TrimSpace(c.From) != ""
}

func DefaultAuth() AuthConfig {
	return AuthConfig{
		SessionCookieName:       "dinchy_session",
		SSOStateCookieName:      "dinchy_sso_state",
		SessionIdleTimeout:      30 * time.Minute,
		SessionMaxLifetime:      7 * 24 * time.Hour,
		SSOStateLifetime:        10 * time.Minute,
		PasswordResetLifetime:   time.Hour,
		TOTPIssuer:              "Dinchy",
		DefaultOrganisationName: "Default",
		DefaultOrganisationSlug: "default",
	}
}

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
	// Auth contains authentication behavior and lifetime settings.
	Auth AuthConfig
	// SSO contains startup SSO provider values loaded from environment.
	SSO SSOEnvConfig
	// SMTP contains outbound email settings for password reset and invitation flows.
	SMTP SMTPConfig
	// Cache contains optional cache store settings for ephemeral state.
	Cache CacheConfig
	// SSOProviders contains the enabled SSO providers derived from env configuration.
	SSOProviders []SSOProviderConfig
}

// defaultConfig returns a Config pre-populated with sensible defaults.
func defaultConfig() Config {
	return Config{
		Addr:         ":8080",
		InternalAddr: ":9090",
		DBBackend:    "sqlite",
		DBPath:       "./dinchy.db",
		DevProxyURL:  "http://127.0.0.1:5173",
		Auth:         DefaultAuth(),
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
	if err := loadFromEnv(&cfg.Auth); err != nil {
		return Config{}, err
	}
	if err := loadFromEnv(&cfg.SSO); err != nil {
		return Config{}, err
	}
	if err := loadFromEnv(&cfg.SMTP); err != nil {
		return Config{}, err
	}
	if err := loadFromEnv(&cfg.Cache); err != nil {
		return Config{}, err
	}
	cfg.SSOProviders = configuredSSOProviders(cfg)

	v := validation.New()
	if err := v.Struct(cfg); err != nil {
		return Config{}, err
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
func loadFromEnv(cfg any) error {
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
		case reflect.Int:
			var parsed int
			if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil {
				return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
					apperrors.WithCause(fmt.Errorf("parse integer env %q for %q: %w", envKey, field.Name, err)),
					apperrors.WithField(apperrors.FieldName(field.Name)),
					apperrors.WithKind(apperrors.FieldKindOf(field.Type.Kind())),
				)
			}
			v.Field(i).SetInt(int64(parsed))
		case reflect.Int64:
			duration, err := time.ParseDuration(raw)
			if err != nil {
				return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
					apperrors.WithCause(fmt.Errorf("parse duration env %q for %q: %w", envKey, field.Name, err)),
					apperrors.WithField(apperrors.FieldName(field.Name)),
					apperrors.WithKind(apperrors.FieldKindOf(field.Type.Kind())),
				)
			}
			v.Field(i).SetInt(int64(duration))
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
