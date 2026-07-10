package config

import "time"

// SessionConfig holds session cookie naming and lifetime settings.
type SessionConfig struct {
	// SessionCookieName is the HTTP cookie name used for Dinchy session tokens.
	SessionCookieName string `env:"DINCHY_AUTH_SESSION_COOKIE_NAME" validate:"required"`
	// SessionIdleTimeout is the maximum idle time before a session is considered expired.
	SessionIdleTimeout time.Duration `env:"DINCHY_AUTH_SESSION_IDLE_TIMEOUT" validate:"gt=0"`
	// SessionMaxLifetime is the absolute maximum age of a session regardless of activity.
	SessionMaxLifetime time.Duration `env:"DINCHY_AUTH_SESSION_MAX_LIFETIME" validate:"gt=0"`
}

// DefaultSession returns the default session configuration used when no environment overrides are provided.
func DefaultSession() SessionConfig {
	return SessionConfig{
		SessionCookieName:  "dinchy_session",
		SessionIdleTimeout: 30 * time.Minute,
		SessionMaxLifetime: 7 * 24 * time.Hour,
	}
}
