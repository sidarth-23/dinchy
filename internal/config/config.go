// IMPORTANT: This file keeps a few startup-only diagnostic literals.
// They are internal failure details only and are never returned to users.

// Package config loads application startup configuration from environment variables.
package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"strings"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// Config holds all startup configuration values for the Dinchy server.
type Config struct {
	// Addr is the plaintext listen address for the main HTTP server. Caddy terminates
	// TLS and proxies here, and Load rejects a non-loopback value: the app trusts the
	// forwarded client address and carries no transport check of its own, so restricting
	// who can connect at all is the boundary that replaces one.
	Addr string `env:"DINCHY_ADDR" validate:"required"`
	// InternalAddr is the listen address for internal health/ready endpoints.
	InternalAddr string `env:"DINCHY_INTERNAL_ADDR" validate:"required"`
	// SecureCookies marks every cookie Secure. It is deployment-wide rather than
	// per-request because cookies ignore port and scheme: a non-Secure cookie minted
	// over plaintext replaces the Secure one of the same name. Only disable it to reach
	// the app directly over plaintext without Caddy.
	SecureCookies bool `env:"DINCHY_SECURE_COOKIES"`
	// Database contains persistent storage backend settings.
	Database DatabaseConfig
	// DevMode enables development mode (relaxed CSP, frontend proxy).
	DevMode bool `env:"DINCHY_DEV"`
	// DevProxyURL is the Vite dev server URL Caddy forwards web requests to in dev mode.
	// Required when DevMode is true. Dinchy never proxies to it: Caddy does, so the
	// browser reaches Vite through the panel hostname and stays same-origin.
	DevProxyURL string `env:"DINCHY_DEV_PROXY_URL" validate:"required_if=DevMode true,omitempty,http_url"`
	// FrontendRoot is the directory Caddy serves the compiled web UI from outside dev
	// mode. The assets never pass through Dinchy.
	FrontendRoot string `env:"DINCHY_FRONTEND_ROOT" mod:"trim" validate:"required"`
	// ExposeInternalErrors adds a debug object to every error response carrying the
	// internal code, cause chain (including SQL errors), and metadata. It leaks internal
	// detail and must stay disabled outside local or trusted debugging environments.
	ExposeInternalErrors bool `env:"DINCHY_EXPOSE_INTERNAL_ERRORS"`
	// PublicBaseURL is the externally reachable base URL used to build links in
	// outbound email (invitation and password reset). Required when SMTP is enabled.
	PublicBaseURL string `env:"DINCHY_PUBLIC_BASE_URL" validate:"omitempty,http_url"`
	// Caddy contains the reverse proxy settings; Caddy owns TLS termination.
	Caddy CaddyConfig
	// Session contains session cookie naming and lifetime settings.
	Session SessionConfig
	// Auth contains authentication behavior and lifetime settings.
	Auth AuthConfig
	// SSO contains startup SSO provider values loaded from environment.
	SSO SSOEnvConfig
	// SMTP contains outbound email settings for password reset and invitation flows.
	SMTP SMTPConfig
	// Redis contains the shared Redis backend settings for ephemeral state and durable event streams.
	Redis RedisConfig
	// Cache contains the optional read-through cache settings.
	Cache CacheConfig
	// EventBus contains the Redis stream settings for durable in-app events.
	EventBus EventBusConfig
	// Worker contains the background job scheduler settings.
	Worker WorkerConfig
	// Jobs contains the durable background job queue settings.
	Jobs JobsConfig
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
		return Config{}, apperrors.Internal(i18n.Msg(i18n.CodePlatformConfigLoadFailed), apperrors.WithCause(err))
	}

	cfg := Config{
		Addr:          "127.0.0.1:8080",
		InternalAddr:  "127.0.0.1:9090",
		SecureCookies: true,
		DevProxyURL:   "http://127.0.0.1:5173",
		FrontendRoot:  "web/dist",
		Caddy:         DefaultCaddy(),
		Database:      DefaultDatabase(),
		Session:       DefaultSession(),
		Auth:          DefaultAuth(),
		SMTP:          DefaultSMTP(),
		Redis:         DefaultRedis(),
		Cache:         DefaultCache(),
		EventBus:      DefaultEventBus(),
		Worker:        DefaultWorker(),
		Jobs:          DefaultJobs(),
		Logging:       DefaultLogging(),
		Telemetry:     DefaultTelemetry(),
	}
	if err := loadFromEnvValue(reflect.ValueOf(&cfg).Elem()); err != nil {
		return Config{}, err
	}
	if err := applyModsValue(reflect.ValueOf(&cfg).Elem()); err != nil {
		return Config{}, err
	}
	cfg.SSOProviders = configuredSSOProviders(cfg)

	if err := validateStruct(cfg); err != nil {
		return Config{}, err
	}

	if cfg.SMTP.Enabled() && cfg.PublicBaseURL == "" {
		return Config{}, apperrors.Internal(i18n.Msg(i18n.CodePlatformConfigValidationFailed), apperrors.WithCause(fmt.Errorf("public base URL is required when SMTP is configured")))
	}

	if err := validateLoopbackAddr(cfg.Addr); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validateLoopbackAddr rejects a public listen address.
//
// Caddy terminates TLS and is the only intended client, so the app trusts the forwarded
// client address and no longer carries a transport check of its own. Restricting the
// listener to loopback is what makes that safe, which makes this a hard requirement
// rather than a recommendation.
func validateLoopbackAddr(addr string) error {
	invalid := func(reason error) error {
		return apperrors.Internal(
			i18n.Msg(i18n.CodePlatformConfigValidationFailed),
			apperrors.WithCause(reason),
		)
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return invalid(fmt.Errorf("DINCHY_ADDR %q is not a host:port address: %w", addr, err))
	}
	if host == "" {
		return invalid(fmt.Errorf("DINCHY_ADDR %q binds every interface; it must be a loopback address such as 127.0.0.1:8080, because Caddy is the only intended client", addr))
	}
	parsed, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return invalid(fmt.Errorf("DINCHY_ADDR host %q is not an IP address; use a loopback address such as 127.0.0.1:8080", host))
	}
	if !parsed.IsLoopback() {
		return invalid(fmt.Errorf("DINCHY_ADDR host %q is not a loopback address; Caddy terminates TLS and proxies to this listener, which must not be reachable from the network", host))
	}
	return nil
}

// FrontendUpstream returns the host:port Caddy forwards web requests to in dev mode,
// derived from DevProxyURL. Empty when the URL cannot be parsed, which validation on
// DevProxyURL already prevents.
func (c Config) FrontendUpstream() string {
	parsed, err := url.Parse(c.DevProxyURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// PublicScheme returns the scheme users reach the app on, derived from PublicBaseURL.
//
// The request cannot supply this: Caddy terminates TLS and proxies plaintext, so every
// request arrives as HTTP regardless of how the browser reached Caddy. It defaults to
// https because Caddy always serves HTTPS.
func (c Config) PublicScheme() string {
	if parsed, err := url.Parse(c.PublicBaseURL); err == nil && parsed.Scheme != "" {
		return parsed.Scheme
	}
	return "https"
}
