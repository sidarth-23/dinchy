package config

import "time"

// PasswordResetMinimumDuration is the minimum wall-clock duration a password reset request should take.
const PasswordResetMinimumDuration = 250 * time.Millisecond

// TOTPFailureLimit is the number of failed TOTP attempts allowed before lockout.
const TOTPFailureLimit int64 = 5

// TOTPLockDuration is the duration of the TOTP lockout after repeated failures.
const TOTPLockDuration = 15 * time.Minute

// AuthConfig holds authentication behavior, cookie names, and token lifetimes.
type AuthConfig struct {
	// SSOStateCookieName is the temporary HTTP cookie name used during SSO redirects.
	SSOStateCookieName string `env:"DINCHY_AUTH_SSO_STATE_COOKIE_NAME" validate:"required"`
	// SSOStateLifetime is the maximum age of a pending SSO redirect transaction.
	SSOStateLifetime time.Duration `env:"DINCHY_AUTH_SSO_STATE_LIFETIME" validate:"gt=0"`
	// PasswordResetLifetime is the validity window for password reset tokens.
	PasswordResetLifetime time.Duration `env:"DINCHY_AUTH_PASSWORD_RESET_LIFETIME" validate:"gt=0"`
	// InviteLifetime is the validity window for organization invitation tokens.
	InviteLifetime time.Duration `env:"DINCHY_AUTH_INVITE_LIFETIME" validate:"gt=0"`
	// TOTPIssuer is the issuer label shown in authenticator apps for Dinchy TOTP secrets.
	TOTPIssuer string `env:"DINCHY_AUTH_TOTP_ISSUER" validate:"required"`
	// DefaultOrganisationName is the organization name created during first-user setup.
	DefaultOrganisationName string `env:"DINCHY_AUTH_DEFAULT_ORGANIZATION_NAME" validate:"required"`
	// DefaultOrganisationSlug is the organization slug created during first-user setup.
	DefaultOrganisationSlug string `env:"DINCHY_AUTH_DEFAULT_ORGANIZATION_SLUG" validate:"required"`
}

// DefaultAuth returns the default authentication configuration used when no
// environment overrides are provided.
func DefaultAuth() AuthConfig {
	return AuthConfig{
		SSOStateCookieName:      "dinchy_sso_state",
		SSOStateLifetime:        10 * time.Minute,
		PasswordResetLifetime:   time.Hour,
		InviteLifetime:          7 * 24 * time.Hour,
		TOTPIssuer:              "Dinchy",
		DefaultOrganisationName: "Default",
		DefaultOrganisationSlug: "default",
	}
}
