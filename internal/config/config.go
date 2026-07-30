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
	// Addr is the plaintext listen address for the main HTTP server. Caddy terminates TLS
	// and proxies here. It may face the network the edge reaches it on — a container
	// address rather than loopback — and TrustedProxies is what keeps the forwarded client
	// address trustworthy when it does.
	Addr string `env:"DINCHY_ADDR" validate:"required"`
	// TrustedProxies lists the CIDR blocks, or bare addresses, whose forwarded headers the
	// app honors, comma-separated. A peer outside it has its X-Forwarded-For ignored, so
	// the address recorded in audit rows is one that peer could not choose. The default
	// covers loopback alone, which is what a host-native deployment behind Caddy needs.
	TrustedProxies string `env:"DINCHY_TRUSTED_PROXIES" mod:"trim"`
	// TrustedProxyPrefixes is TrustedProxies parsed, derived at Load like SSOProviders. It
	// carries no env tag on purpose: the env walker handles scalars only, so a tagged slice
	// would fail to load rather than being skipped.
	TrustedProxyPrefixes []netip.Prefix
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
	// FrontendURL is the URL the edge forwards web requests to: the Vite dev server in
	// development, a static file server alongside the app otherwise. The assets never pass
	// through Dinchy, and never through the edge's filesystem — the edge is shared between
	// applications and can see none of their files.
	FrontendURL string `env:"DINCHY_FRONTEND_URL" validate:"required,http_url"`
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
		Addr:           "127.0.0.1:8080",
		TrustedProxies: "127.0.0.1/32,::1/128",
		InternalAddr:   "127.0.0.1:9090",
		SecureCookies:  true,
		FrontendURL:    "http://127.0.0.1:3000",
		Caddy:          DefaultCaddy(),
		Database:       DefaultDatabase(),
		Session:        DefaultSession(),
		Auth:           DefaultAuth(),
		SMTP:           DefaultSMTP(),
		Redis:          DefaultRedis(),
		Cache:          DefaultCache(),
		EventBus:       DefaultEventBus(),
		Worker:         DefaultWorker(),
		Jobs:           DefaultJobs(),
		Logging:        DefaultLogging(),
		Telemetry:      DefaultTelemetry(),
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

	trustedProxies, err := validateForwardedTrust(cfg)
	if err != nil {
		return Config{}, err
	}
	cfg.TrustedProxyPrefixes = trustedProxies

	return cfg, nil
}

// validateForwardedTrust parses the trusted proxy set and rejects a listener that set cannot
// account for.
//
// The app records the forwarded client address in audit rows and honors it only from a trusted
// peer, so a listener reachable beyond loopback while no non-loopback proxy is named would record
// the proxy's own address for every request. That is wrong in a way an operator cannot see, which
// is why it fails at startup rather than being left to a reader of the audit log.
//
// The converse — a loopback listener with wider trust — is harmless and is not policed: nothing
// outside loopback can reach the listener to exercise it.
func validateForwardedTrust(cfg Config) ([]netip.Prefix, error) {
	invalid := func(reason error) error {
		return apperrors.Internal(
			i18n.Msg(i18n.CodePlatformConfigValidationFailed),
			apperrors.WithCause(reason),
		)
	}

	prefixes, err := parseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		return nil, invalid(err)
	}

	reachable, err := listenerIsReachable(cfg.Addr)
	if err != nil {
		return nil, invalid(err)
	}
	if !reachable {
		return prefixes, nil
	}

	for _, prefix := range prefixes {
		if !prefix.Addr().IsLoopback() {
			return prefixes, nil
		}
	}
	return nil, invalid(fmt.Errorf(
		"DINCHY_ADDR %q is reachable beyond loopback but DINCHY_TRUSTED_PROXIES %q names only loopback; add the network the reverse proxy reaches this listener from, or bind a loopback address",
		cfg.Addr, cfg.TrustedProxies))
}

// parseTrustedProxies turns the configured list into prefixes, accepting a bare address as the
// single-address prefix it means. Accepting both spellings removes a class of startup failure
// whose message says nothing about the missing "/32".
func parseTrustedProxies(configured string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for entry := range strings.SplitSeq(configured, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		address, err := netip.ParseAddr(entry)
		if err != nil {
			return nil, fmt.Errorf("DINCHY_TRUSTED_PROXIES entry %q is neither a CIDR block nor an IP address", entry)
		}
		prefixes = append(prefixes, netip.PrefixFrom(address, address.BitLen()))
	}
	return prefixes, nil
}

// listenerIsReachable reports whether the listen address accepts connections from beyond
// loopback. A host that is not an IP address counts as reachable: it cannot be classified here,
// and treating the unknown as exposed is the direction that fails safe.
func listenerIsReachable(addr string) (bool, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false, fmt.Errorf("DINCHY_ADDR %q is not a host:port address: %w", addr, err)
	}
	if host == "" {
		return true, nil
	}
	parsed, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return true, nil
	}
	return !parsed.IsLoopback(), nil
}

// FrontendUpstream returns the host:port the edge forwards web requests to, derived from
// FrontendURL. Empty when the URL cannot be parsed, which validation on FrontendURL already
// prevents.
func (c Config) FrontendUpstream() string {
	parsed, err := url.Parse(c.FrontendURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// PanelUpstream returns the host:port the edge dials to reach the API.
//
// It is not the listen address, though it falls back to it. A container listens on every interface
// and is reached by name, and an address the app binds is not necessarily one the edge can route
// to — so the advertised address is configuration in its own right.
func (c Config) PanelUpstream() string {
	if c.Caddy.PanelUpstream != "" {
		return c.Caddy.PanelUpstream
	}
	return c.Addr
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
