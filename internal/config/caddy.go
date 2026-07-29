package config

import (
	"time"
)

// TLS issuer names for CaddyConfig.TLSIssuer. They are Caddy's own module identifiers,
// passed through to the certificate automation policy unchanged.
const (
	// TLSIssuerACME obtains certificates from a public authority over ACME.
	TLSIssuerACME = "acme"
	// TLSIssuerInternal signs certificates with Caddy's own local CA.
	TLSIssuerInternal = "internal"
)

// CaddyConfig holds the settings for the Caddy reverse proxy that fronts Dinchy.
// Caddy is the only TLS terminator: Dinchy always serves plaintext on loopback and
// Caddy owns certificates, automatic HTTPS, and HSTS.
//
// An empty environment variable keeps the default below, so a value that must differ
// from its default has to be set explicitly — writing DINCHY_CADDY_ENABLED= leaves
// management on rather than disabling it.
type CaddyConfig struct {
	// Enabled turns on Caddy configuration management. When false Dinchy pushes
	// nothing and an operator drives Caddy themselves.
	Enabled bool `env:"DINCHY_CADDY_ENABLED"`
	// AdminEndpoint is the host:port of Caddy's admin API.
	AdminEndpoint string `env:"DINCHY_CADDY_ADMIN_ENDPOINT" mod:"trim" validate:"required,hostname_port"`
	// AdminTimeout bounds a single admin API call. Loading a configuration can block
	// on certificate provisioning, so this is generous by default.
	AdminTimeout time.Duration `env:"DINCHY_CADDY_ADMIN_TIMEOUT" validate:"gt=0"`
	// HTTPSPort is the port Caddy serves HTTPS on.
	HTTPSPort uint16 `env:"DINCHY_CADDY_HTTPS_PORT" validate:"gt=0"`
	// PanelHost is the hostname Caddy serves the Dinchy panel on.
	PanelHost string `env:"DINCHY_CADDY_PANEL_HOST" mod:"trim,lower" validate:"required,hostname_rfc1123"`
	// TLSIssuer selects where certificates come from: TLSIssuerACME for a public domain,
	// or TLSIssuerInternal for Caddy's own local CA. Development uses the local CA
	// because no public authority can validate localhost; `caddy trust` installs its root
	// so the browser accepts it without a warning.
	TLSIssuer string `env:"DINCHY_CADDY_TLS_ISSUER" mod:"trim,lower" validate:"required,oneof=acme internal"`
	// ACMEEmail is the contact address registered with the ACME certificate authority.
	ACMEEmail string `env:"DINCHY_CADDY_ACME_EMAIL" mod:"trim" validate:"omitempty,email"`
	// ACMECA overrides the ACME directory URL, for a staging or private authority.
	ACMECA string `env:"DINCHY_CADDY_ACME_CA" mod:"trim" validate:"omitempty,http_url"`
	// StoragePath overrides where Caddy keeps certificates and ACME account keys. Empty
	// leaves Caddy's own XDG-based location, which deploy/systemd/caddy.service pins.
	// Wherever it points must be persistent: if it moves, Caddy re-registers with the
	// certificate authority and re-issues everything, which is a fast way to hit
	// Let's Encrypt's per-domain rate limit.
	StoragePath string `env:"DINCHY_CADDY_STORAGE_PATH" mod:"trim"`
	// HSTSMaxAge sets Strict-Transport-Security on proxied responses. Zero omits the
	// header, which is the right default for local development — pinning HSTS on
	// localhost would affect every other plaintext server on the same host.
	HSTSMaxAge time.Duration `env:"DINCHY_CADDY_HSTS_MAX_AGE" validate:"gte=0"`
	// HSTSIncludeSubdomains adds includeSubDomains to Strict-Transport-Security.
	HSTSIncludeSubdomains bool `env:"DINCHY_CADDY_HSTS_INCLUDE_SUBDOMAINS"`
}

// DefaultCaddy returns the production-shaped Caddy configuration used when no
// environment overrides are provided.
func DefaultCaddy() CaddyConfig {
	return CaddyConfig{
		Enabled:               true,
		AdminEndpoint:         "127.0.0.1:2019",
		AdminTimeout:          30 * time.Second,
		HTTPSPort:             443,
		PanelHost:             "localhost",
		TLSIssuer:             TLSIssuerACME,
		HSTSMaxAge:            365 * 24 * time.Hour,
		HSTSIncludeSubdomains: true,
	}
}

// UsesLocalCA reports whether Caddy signs its own certificates instead of obtaining them
// from a public authority.
func (c CaddyConfig) UsesLocalCA() bool {
	return c.TLSIssuer == TLSIssuerInternal
}
