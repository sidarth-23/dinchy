package config

import "time"

type AuthConfig struct {
	// SessionCookieName is the HTTP cookie name used for Dinchy session tokens.
	SessionCookieName string `env:"DINCHY_AUTH_SESSION_COOKIE_NAME" validate:"required"`
	// SSOStateCookieName is the temporary HTTP cookie name used during SSO redirects.
	SSOStateCookieName string `env:"DINCHY_AUTH_SSO_STATE_COOKIE_NAME" validate:"required"`
	// SessionIdleTimeout is the maximum idle time before a session is considered expired.
	SessionIdleTimeout time.Duration `env:"DINCHY_AUTH_SESSION_IDLE_TIMEOUT" validate:"gt=0"`
	// SessionMaxLifetime is the absolute maximum age of a session regardless of activity.
	SessionMaxLifetime time.Duration `env:"DINCHY_AUTH_SESSION_MAX_LIFETIME" validate:"gt=0"`
	// SSOStateLifetime is the maximum age of a pending SSO redirect transaction.
	SSOStateLifetime time.Duration `env:"DINCHY_AUTH_SSO_STATE_LIFETIME" validate:"gt=0"`
	// PasswordResetLifetime is the validity window for password reset tokens.
	PasswordResetLifetime time.Duration `env:"DINCHY_AUTH_PASSWORD_RESET_LIFETIME" validate:"gt=0"`
	// InviteLifetime is the validity window for organization invitation tokens.
	InviteLifetime time.Duration `env:"DINCHY_AUTH_INVITE_LIFETIME" validate:"gt=0"`
	// TOTPIssuer is the issuer label shown in authenticator apps for Dinchy TOTP secrets.
	TOTPIssuer string `env:"DINCHY_AUTH_TOTP_ISSUER" validate:"required"`
	// DefaultOrganisationName is the organisation name created during first-user setup.
	DefaultOrganisationName string `env:"DINCHY_AUTH_DEFAULT_ORGANISATION_NAME" validate:"required"`
	// DefaultOrganisationSlug is the organisation slug created during first-user setup.
	DefaultOrganisationSlug string `env:"DINCHY_AUTH_DEFAULT_ORGANISATION_SLUG" validate:"required"`
}

func DefaultAuth() AuthConfig {
	return AuthConfig{
		SessionCookieName:       "dinchy_session",
		SSOStateCookieName:      "dinchy_sso_state",
		SessionIdleTimeout:      30 * time.Minute,
		SessionMaxLifetime:      7 * 24 * time.Hour,
		SSOStateLifetime:        10 * time.Minute,
		PasswordResetLifetime:   time.Hour,
		InviteLifetime:          7 * 24 * time.Hour,
		TOTPIssuer:              "Dinchy",
		DefaultOrganisationName: "Default",
		DefaultOrganisationSlug: "default",
	}
}
