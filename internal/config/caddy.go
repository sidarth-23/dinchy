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

// CaddyConfig holds the settings for the shared Caddy edge that fronts Dinchy.
//
// The edge is not Dinchy's: it owns its own listeners, ports, certificate storage and admin
// endpoint, and it fronts every application on the host. Dinchy is one tenant, and writes only
// the two objects it owns — see internal/platform/caddy.
//
// Caddy is still the only TLS terminator; Dinchy serves plaintext and owns no certificate.
//
// An empty environment variable keeps the default below, so a value that must differ
// from its default has to be set explicitly — writing DINCHY_CADDY_ENABLED= leaves
// management on rather than disabling it.
type CaddyConfig struct {
	// Enabled turns on Caddy configuration management. When false Dinchy pushes
	// nothing and an operator drives Caddy themselves.
	Enabled bool `env:"DINCHY_CADDY_ENABLED"`
	// AdminEndpoint is the host:port of the edge's admin API.
	//
	// The admin API is unauthenticated, so whichever network carries it is a trust boundary:
	// anything that can reach it can reconfigure the whole edge, including other tenants'
	// routes. Keep it off any network wider than the one the applications share.
	AdminEndpoint string `env:"DINCHY_CADDY_ADMIN_ENDPOINT" mod:"trim" validate:"required,hostname_port"`
	// AdminTimeout bounds a single admin API call. Applying a configuration can block
	// on certificate provisioning, so this is generous by default.
	AdminTimeout time.Duration `env:"DINCHY_CADDY_ADMIN_TIMEOUT" validate:"gt=0"`
	// EdgeServerName is the key of the edge's HTTP server this deployment writes its route
	// into. The edge's base configuration creates it; Dinchy never creates or replaces it.
	EdgeServerName string `env:"DINCHY_CADDY_EDGE_SERVER" mod:"trim" validate:"required"`
	// Tenant namespaces the configuration objects this deployment owns, so two Dinchy
	// instances behind one edge address their own and never each other's.
	Tenant string `env:"DINCHY_CADDY_TENANT" mod:"trim" validate:"required"`
	// PanelHost is the hostname Caddy serves the Dinchy panel on.
	PanelHost string `env:"DINCHY_CADDY_PANEL_HOST" mod:"trim,lower" validate:"required,hostname_rfc1123"`
	// PanelUpstream is the host:port the edge dials to reach this app's API, when that
	// differs from the listen address. Empty falls back to Addr.
	//
	// The two are different values: a container listens on 0.0.0.0 and is dialed by name,
	// and an address the app binds is not necessarily one the edge can reach.
	PanelUpstream string `env:"DINCHY_CADDY_PANEL_UPSTREAM" mod:"trim" validate:"omitempty,hostname_port"`
	// TLSIssuer selects where certificates come from: TLSIssuerACME for a public domain,
	// or TLSIssuerInternal for Caddy's own local CA. Development uses the local CA
	// because no public authority can validate localhost; `caddy trust` installs its root
	// so the browser accepts it without a warning.
	TLSIssuer string `env:"DINCHY_CADDY_TLS_ISSUER" mod:"trim,lower" validate:"required,oneof=acme internal"`
	// ACMEEmail is the contact address registered with the ACME certificate authority.
	ACMEEmail string `env:"DINCHY_CADDY_ACME_EMAIL" mod:"trim" validate:"omitempty,email"`
	// ACMECA overrides the ACME directory URL, for a staging or private authority.
	ACMECA string `env:"DINCHY_CADDY_ACME_CA" mod:"trim" validate:"omitempty,http_url"`
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
		EdgeServerName:        "edge",
		Tenant:                "dinchy",
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
