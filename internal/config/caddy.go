package config

import (
	"fmt"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// CaddyConfig holds the settings for the Caddy reverse proxy that fronts Dinchy.
// Caddy is the only TLS terminator: Dinchy always serves plaintext on loopback and
// Caddy owns certificates, automatic HTTPS, and HSTS.
//
// An empty environment variable keeps the default below, so a value that must differ
// from its default has to be set explicitly — writing DINCHY_CADDY_AUTOMATIC_HTTPS=
// leaves automatic HTTPS on rather than disabling it.
type CaddyConfig struct {
	// Enabled turns on Caddy configuration management. When false Dinchy pushes
	// nothing and an operator drives Caddy themselves.
	Enabled bool `env:"DINCHY_CADDY_ENABLED"`
	// AdminEndpoint is the host:port of Caddy's admin API.
	AdminEndpoint string `env:"DINCHY_CADDY_ADMIN_ENDPOINT" mod:"trim" validate:"required,hostname_port"`
	// AdminTimeout bounds a single admin API call. Loading a configuration can block
	// on certificate provisioning, so this is generous by default.
	AdminTimeout time.Duration `env:"DINCHY_CADDY_ADMIN_TIMEOUT" validate:"gt=0"`
	// Binary is the path to the Caddy executable, used to read the compiled module
	// set so Dinchy only offers plugins that are actually available.
	Binary string `env:"DINCHY_CADDY_BINARY" mod:"trim" validate:"required"`
	// ReconcileInterval is how often the drift job re-pushes the desired routes.
	ReconcileInterval time.Duration `env:"DINCHY_CADDY_RECONCILE_INTERVAL" validate:"gt=0"`
	// HTTPSPort is the port Caddy serves HTTPS on.
	HTTPSPort uint16 `env:"DINCHY_CADDY_HTTPS_PORT" validate:"gt=0"`
	// PanelHost is the hostname Caddy serves the Dinchy panel on. No deployment
	// route may claim it.
	PanelHost string `env:"DINCHY_CADDY_PANEL_HOST" mod:"trim,lower" validate:"required,hostname_rfc1123"`
	// AutomaticHTTPS lets Caddy obtain certificates over ACME. Disable it to serve
	// CertFile and KeyFile instead, as local development does with mkcert.
	AutomaticHTTPS bool `env:"DINCHY_CADDY_AUTOMATIC_HTTPS"`
	// CertFile is the PEM certificate served when AutomaticHTTPS is false.
	CertFile string `env:"DINCHY_CADDY_CERT_FILE" mod:"trim"`
	// KeyFile is the PEM private key served when AutomaticHTTPS is false.
	KeyFile string `env:"DINCHY_CADDY_KEY_FILE" mod:"trim"`
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
		Binary:                "/usr/local/bin/caddy",
		ReconcileInterval:     time.Minute,
		HTTPSPort:             443,
		PanelHost:             "localhost",
		AutomaticHTTPS:        true,
		HSTSMaxAge:            365 * 24 * time.Hour,
		HSTSIncludeSubdomains: true,
	}
}

// ServesOwnCertificate reports whether Caddy serves CertFile and KeyFile rather than
// obtaining certificates over ACME.
func (c CaddyConfig) ServesOwnCertificate() bool {
	return !c.AutomaticHTTPS
}

// validate reports configuration that would leave Caddy unable to serve TLS.
func (c CaddyConfig) validate() error {
	if !c.Enabled || c.AutomaticHTTPS {
		return nil
	}
	if c.CertFile == "" || c.KeyFile == "" {
		return apperrors.Internal(
			i18n.Msg(i18n.CodePlatformConfigValidationFailed),
			apperrors.WithCause(fmt.Errorf("DINCHY_CADDY_CERT_FILE and DINCHY_CADDY_KEY_FILE are required when DINCHY_CADDY_AUTOMATIC_HTTPS is false")),
		)
	}
	return nil
}
