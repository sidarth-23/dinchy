package config_test

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	clearDinchyEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:8080", cfg.Addr)
	assert.Equal(t, "127.0.0.1:9090", cfg.InternalAddr)
	assert.Equal(t, "postgres://postgres:postgres@localhost:5432/dinchy?sslmode=disable", cfg.Database.PostgresDSN)
	assert.Equal(t, "http://127.0.0.1:3000", cfg.FrontendURL)
	assert.False(t, cfg.DevMode)
	assert.True(t, cfg.Caddy.Enabled)
	assert.Equal(t, "127.0.0.1:2019", cfg.Caddy.AdminEndpoint)
	assert.Equal(t, "edge", cfg.Caddy.EdgeServerName)
	assert.Equal(t, "dinchy", cfg.Caddy.Tenant)
	assert.Equal(t, config.TLSIssuerACME, cfg.Caddy.TLSIssuer)
	assert.False(t, cfg.Caddy.UsesLocalCA())
	assert.Equal(t, "dinchy_session", cfg.Session.SessionCookieName)
	assert.Equal(t, "dinchy_sso_state", cfg.Auth.SSOStateCookieName)
	assert.Equal(t, 30*time.Minute, cfg.Session.SessionIdleTimeout)
	assert.Equal(t, 7*24*time.Hour, cfg.Session.SessionMaxLifetime)
	assert.Equal(t, time.Hour, cfg.Auth.PasswordResetLifetime)
	assert.Equal(t, 7*24*time.Hour, cfg.Auth.InviteLifetime)
	assert.Equal(t, "127.0.0.1:6379", cfg.Redis.Addr)
	assert.Equal(t, 0, cfg.Redis.Database)
	assert.Equal(t, "dinchy", cfg.Redis.KeyPrefix)
	assert.Equal(t, "app.events", cfg.EventBus.StreamName)
	assert.Equal(t, "app", cfg.EventBus.ConsumerGroupPrefix)
	assert.Equal(t, "local", cfg.EventBus.ConsumerName)
	assert.Equal(t, 100, cfg.EventBus.BatchSize)
	assert.Equal(t, 5*time.Minute, cfg.EventBus.RetentionWindow)
	assert.Equal(t, 2*time.Minute, cfg.EventBus.ClaimMinIdle)
	assert.Equal(t, 500*time.Millisecond, cfg.EventBus.ReadBlock)
	assert.Equal(t, 5*time.Second, cfg.EventBus.WorkerInterval)
	assert.Equal(t, uint16(587), cfg.SMTP.Port)
	assert.Empty(t, cfg.SSOProviders)
	assert.False(t, cfg.SMTP.Enabled())
}

func TestLoad_AllOverrides(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ADDR", "127.0.0.1:9999")
	t.Setenv("DINCHY_INTERNAL_ADDR", "127.0.0.1:8888")
	t.Setenv("DINCHY_POSTGRES_DSN", "postgres://test:test@localhost:5432/dinchy?sslmode=disable")
	t.Setenv("DINCHY_DEV", "true")
	t.Setenv("DINCHY_FRONTEND_URL", "http://localhost:4000")
	t.Setenv("DINCHY_GITHUB_CLIENT_ID", "client")
	t.Setenv("DINCHY_GITHUB_CLIENT_SECRET", "secret")
	t.Setenv("DINCHY_GITHUB_CALLBACK_URL", "https://app.example.com/api/auth/sso/github/callback")
	t.Setenv("DINCHY_AUTH_SESSION_COOKIE_NAME", "custom_session")
	t.Setenv("DINCHY_AUTH_SESSION_IDLE_TIMEOUT", "45m")
	t.Setenv("DINCHY_AUTH_INVITE_LIFETIME", "72h")
	t.Setenv("DINCHY_REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("DINCHY_REDIS_DATABASE", "2")
	t.Setenv("DINCHY_REDIS_KEY_PREFIX", "dinchy-test")
	t.Setenv("DINCHY_EVENT_BUS_STREAM_NAME", "audit.events")
	t.Setenv("DINCHY_EVENT_BUS_CONSUMER_GROUP_PREFIX", "audit")
	t.Setenv("DINCHY_EVENT_BUS_CONSUMER_NAME", "worker-a")
	t.Setenv("DINCHY_EVENT_BUS_BATCH_SIZE", "25")
	t.Setenv("DINCHY_EVENT_BUS_RETENTION_WINDOW", "10m")
	t.Setenv("DINCHY_EVENT_BUS_CLAIM_MIN_IDLE", "3m")
	t.Setenv("DINCHY_EVENT_BUS_READ_BLOCK", "1s")
	t.Setenv("DINCHY_EVENT_BUS_WORKER_INTERVAL", "15s")
	t.Setenv("DINCHY_SMTP_HOST", "smtp.example.com")
	t.Setenv("DINCHY_SMTP_FROM", "dinchy@example.com")
	t.Setenv("DINCHY_PUBLIC_BASE_URL", "https://app.example.com")
	t.Setenv("DINCHY_CADDY_ADMIN_ENDPOINT", "127.0.0.1:3019")
	t.Setenv("DINCHY_CADDY_ADMIN_TIMEOUT", "10s")
	t.Setenv("DINCHY_CADDY_EDGE_SERVER", "shared")
	t.Setenv("DINCHY_CADDY_TENANT", "dinchy-two")
	t.Setenv("DINCHY_CADDY_PANEL_UPSTREAM", "dinchy-api:8080")
	t.Setenv("DINCHY_CADDY_PANEL_HOST", "Panel.Example.COM")
	t.Setenv("DINCHY_CADDY_ACME_EMAIL", "ops@example.com")
	t.Setenv("DINCHY_CADDY_HSTS_MAX_AGE", "48h")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9999", cfg.Addr)
	assert.Equal(t, "127.0.0.1:8888", cfg.InternalAddr)
	assert.Equal(t, "postgres://test:test@localhost:5432/dinchy?sslmode=disable", cfg.Database.PostgresDSN)
	assert.True(t, cfg.DevMode)
	assert.Equal(t, "http://localhost:4000", cfg.FrontendURL)
	require.Len(t, cfg.SSOProviders, 1)
	assert.Equal(t, config.SSOProviderGitHub, cfg.SSOProviders[0].ID)
	assert.Equal(t, "custom_session", cfg.Session.SessionCookieName)
	assert.Equal(t, 45*time.Minute, cfg.Session.SessionIdleTimeout)
	assert.Equal(t, 72*time.Hour, cfg.Auth.InviteLifetime)
	assert.Equal(t, "127.0.0.1:6379", cfg.Redis.Addr)
	assert.Equal(t, 2, cfg.Redis.Database)
	assert.Equal(t, "dinchy-test", cfg.Redis.KeyPrefix)
	assert.Equal(t, "audit.events", cfg.EventBus.StreamName)
	assert.Equal(t, "audit", cfg.EventBus.ConsumerGroupPrefix)
	assert.Equal(t, "worker-a", cfg.EventBus.ConsumerName)
	assert.Equal(t, 25, cfg.EventBus.BatchSize)
	assert.Equal(t, 10*time.Minute, cfg.EventBus.RetentionWindow)
	assert.Equal(t, 3*time.Minute, cfg.EventBus.ClaimMinIdle)
	assert.Equal(t, 1*time.Second, cfg.EventBus.ReadBlock)
	assert.Equal(t, 15*time.Second, cfg.EventBus.WorkerInterval)
	assert.True(t, cfg.SMTP.Enabled())
	assert.Equal(t, "https://app.example.com", cfg.PublicBaseURL)
	assert.Equal(t, "127.0.0.1:3019", cfg.Caddy.AdminEndpoint)
	assert.Equal(t, 10*time.Second, cfg.Caddy.AdminTimeout)
	assert.Equal(t, "shared", cfg.Caddy.EdgeServerName)
	assert.Equal(t, "dinchy-two", cfg.Caddy.Tenant)
	assert.Equal(t, "dinchy-api:8080", cfg.Caddy.PanelUpstream)
	assert.Equal(t, "panel.example.com", cfg.Caddy.PanelHost, "panel host is lowercased so route comparisons are case-insensitive")
	assert.Equal(t, "ops@example.com", cfg.Caddy.ACMEEmail)
	assert.Equal(t, 48*time.Hour, cfg.Caddy.HSTSMaxAge)
}

func TestLoad_DevMode_AcceptsMultipleBoolFormats(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "True"} {
		t.Run(v, func(t *testing.T) {
			clearDinchyEnv(t)
			t.Setenv("DINCHY_DEV", v)
			cfg, err := config.Load()
			require.NoError(t, err)
			assert.True(t, cfg.DevMode, "DINCHY_DEV=%q should enable dev mode", v)
		})
	}
}

func TestLoad_InvalidFrontendURL_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_FRONTEND_URL", "not-a-url")

	_, err := config.Load()
	require.Error(t, err, "an invalid FrontendURL should fail validation")
}

func TestLoad_DevMode_DefaultFrontendURLPassesValidation(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_DEV", "true")
	// FrontendURL intentionally not set — the default must satisfy validation on its own.

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.DevMode)
	assert.Equal(t, "http://127.0.0.1:3000", cfg.FrontendURL)
}

func TestLoad_InvalidLoggingConfig_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_LOG_LEVEL", "verbose")

	_, err := config.Load()
	require.Error(t, err)
}

func TestLoad_TraceSampleRatio_ParsesFloat(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_OTEL_TRACES_SAMPLE_RATIO", "0.25")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.InEpsilon(t, 0.25, cfg.Telemetry.SampleRatio, 1e-9)
}

func TestLoad_InvalidTraceSampleRatio_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_OTEL_TRACES_SAMPLE_RATIO", "not-a-float")

	_, err := config.Load()
	require.Error(t, err)
}

func TestLoad_NormalizesLogLevel(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_LOG_LEVEL", "  INFO  ")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, config.LogLevelInfo, cfg.Logging.Level)
}

func TestLoad_AcceptsTraceLogLevel(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_LOG_LEVEL", "trace")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, config.LogLevelTrace, cfg.Logging.Level)
}

func TestLoad_TrimsConfigFields(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_EVENT_BUS_CONSUMER_GROUP_PREFIX", "  audit  ")
	t.Setenv("DINCHY_GITHUB_CLIENT_ID", "  client  ")
	t.Setenv("DINCHY_GITHUB_CLIENT_SECRET", "  secret  ")
	t.Setenv("DINCHY_GITHUB_CALLBACK_URL", "  https://app.example.com/api/auth/sso/github/callback  ")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "audit", cfg.EventBus.ConsumerGroupPrefix)
	require.Len(t, cfg.SSOProviders, 1)
	assert.Equal(t, "client", cfg.SSOProviders[0].ClientID)
	assert.Equal(t, "https://app.example.com/api/auth/sso/github/callback", cfg.SSOProviders[0].CallbackURL)
}

func TestLoad_InvalidEventBusBatchSize_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_EVENT_BUS_BATCH_SIZE", "0")

	_, err := config.Load()
	require.Error(t, err)
}

func TestLoad_MissingExplicitEnvFile_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ENV_FILE", "/tmp/definitely-does-not-exist-dinchy.env")

	_, err := config.Load()
	require.Error(t, err, "explicit env file that doesn't exist should fail")
}

func TestLoad_Caddy_TLSIssuer(t *testing.T) {
	t.Run("internal selects Caddy's local CA", func(t *testing.T) {
		clearDinchyEnv(t)
		t.Setenv("DINCHY_CADDY_TLS_ISSUER", "Internal")
		cfg, err := config.Load()
		require.NoError(t, err)
		assert.Equal(t, config.TLSIssuerInternal, cfg.Caddy.TLSIssuer, "the value is lowercased")
		assert.True(t, cfg.Caddy.UsesLocalCA())
	})
	t.Run("an unknown issuer fails rather than falling back", func(t *testing.T) {
		clearDinchyEnv(t)
		t.Setenv("DINCHY_CADDY_TLS_ISSUER", "letsencrypt")
		_, err := config.Load()
		require.Error(t, err)
	})
	// Blanking the variable keeps the default, so a developer who wants the local CA must
	// write it explicitly or Caddy will attempt ACME against localhost.
	t.Run("blank keeps the ACME default", func(t *testing.T) {
		clearDinchyEnv(t)
		t.Setenv("DINCHY_CADDY_TLS_ISSUER", "")
		cfg, err := config.Load()
		require.NoError(t, err)
		assert.Equal(t, config.TLSIssuerACME, cfg.Caddy.TLSIssuer)
	})
}

// causeOf returns the internal diagnostic behind a startup failure, which is where the variable
// names live.
func causeOf(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	unwrapped, ok := err.(interface{ Unwrap() error })
	require.True(t, ok, "a startup failure must carry its cause")
	return unwrapped.Unwrap().Error()
}

// TestLoad_RejectsAReachableListenerTrustedOnlyOnLoopback is the invariant that replaced the
// loopback-only rule. Such a listener is not forgeable, but it records the proxy's own address for
// every request — wrong in a way no operator would see, so it fails at startup instead.
func TestLoad_RejectsAReachableListenerTrustedOnlyOnLoopback(t *testing.T) {
	for _, addr := range []string{":8080", "0.0.0.0:8080", "10.0.0.5:8080", "[::]:8080"} {
		t.Run(addr, func(t *testing.T) {
			clearDinchyEnv(t)
			t.Setenv("DINCHY_ADDR", addr)

			_, err := config.Load()

			cause := causeOf(t, err)
			assert.Contains(t, cause, "DINCHY_ADDR")
			assert.Contains(t, cause, "DINCHY_TRUSTED_PROXIES",
				"the message must name both variables, because either one is a valid fix")
		})
	}
}

func TestLoad_AcceptsAReachableListenerWithANonLoopbackTrustedProxy(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ADDR", "0.0.0.0:8080")
	t.Setenv("DINCHY_TRUSTED_PROXIES", "10.89.100.0/24")

	cfg, err := config.Load()

	require.NoError(t, err)
	require.Len(t, cfg.TrustedProxyPrefixes, 1)
	assert.Equal(t, "10.89.100.0/24", cfg.TrustedProxyPrefixes[0].String())
}

func TestLoad_AcceptsLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "127.0.0.2:8080", "[::1]:8080"} {
		t.Run(addr, func(t *testing.T) {
			clearDinchyEnv(t)
			t.Setenv("DINCHY_ADDR", addr)

			_, err := config.Load()

			require.NoError(t, err)
		})
	}
}

func TestLoad_TrustedProxiesDefaultsToLoopback(t *testing.T) {
	clearDinchyEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	// The default is exactly what the old loopback-only listener guaranteed, so an existing
	// host-native deployment keeps recording real client addresses.
	assert.Equal(t, []string{"127.0.0.1/32", "::1/128"}, prefixStrings(cfg.TrustedProxyPrefixes))
}

// TestLoad_TrustedProxiesAcceptsBareAddresses removes a class of startup failure whose message
// would say nothing about a missing "/32".
func TestLoad_TrustedProxiesAcceptsBareAddresses(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_TRUSTED_PROXIES", "127.0.0.1, ::1 ,10.89.100.4")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, []string{"127.0.0.1/32", "::1/128", "10.89.100.4/32"},
		prefixStrings(cfg.TrustedProxyPrefixes))
}

func TestLoad_RejectsAMalformedTrustedProxy(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_TRUSTED_PROXIES", "127.0.0.1/32,not-an-address")

	_, err := config.Load()

	assert.Contains(t, causeOf(t, err), `"not-an-address"`, "the offending value must be quoted")
}

func TestLoad_RejectsAnAddrWithoutAPort(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ADDR", "127.0.0.1")

	_, err := config.Load()

	assert.Contains(t, causeOf(t, err), "DINCHY_ADDR")
}

func prefixStrings(prefixes []netip.Prefix) []string {
	rendered := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		rendered = append(rendered, prefix.String())
	}
	return rendered
}

func TestFrontendUpstream(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_FRONTEND_URL", "http://127.0.0.1:3000")

	cfg, err := config.Load()
	require.NoError(t, err)

	// The edge needs a dial address, not a URL, because it proxies to the frontend itself.
	assert.Equal(t, "127.0.0.1:3000", cfg.FrontendUpstream())
}

// TestPanelUpstream_FallsBackToTheListenAddress pins that the advertised upstream and the listen
// address are separate values, because a container binds one and is dialed by the other.
func TestPanelUpstream_FallsBackToTheListenAddress(t *testing.T) {
	clearDinchyEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:8080", cfg.PanelUpstream())
}

func TestPanelUpstream_PrefersTheConfiguredValue(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ADDR", "0.0.0.0:8080")
	t.Setenv("DINCHY_TRUSTED_PROXIES", "10.89.100.0/24")
	t.Setenv("DINCHY_CADDY_PANEL_UPSTREAM", "dinchy-api:8080")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "dinchy-api:8080", cfg.PanelUpstream())
}

func TestPublicScheme(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"from the public base URL", "https://panel.example.com", "https"},
		{"plaintext base URL", "http://localhost:8080", "http"},
		// Caddy always serves HTTPS, so that is the right answer when unset.
		{"unset defaults to https", "", "https"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearDinchyEnv(t)
			if tt.baseURL != "" {
				t.Setenv("DINCHY_PUBLIC_BASE_URL", tt.baseURL)
			}

			cfg, err := config.Load()
			require.NoError(t, err)

			assert.Equal(t, tt.want, cfg.PublicScheme())
		})
	}
}

// clearDinchyEnv clears all DINCHY_ env vars so tests start from a clean baseline.
func clearDinchyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DINCHY_ADDR", "DINCHY_TRUSTED_PROXIES", "DINCHY_INTERNAL_ADDR", "DINCHY_POSTGRES_DSN",
		"DINCHY_DEV", "DINCHY_FRONTEND_URL",
		"DINCHY_GOOGLE_CLIENT_ID", "DINCHY_GOOGLE_CLIENT_SECRET", "DINCHY_GOOGLE_CALLBACK_URL",
		"DINCHY_GITHUB_CLIENT_ID", "DINCHY_GITHUB_CLIENT_SECRET", "DINCHY_GITHUB_CALLBACK_URL",
		"DINCHY_GITLAB_CLIENT_ID", "DINCHY_GITLAB_CLIENT_SECRET", "DINCHY_GITLAB_CALLBACK_URL",
		"DINCHY_AUTH_SESSION_COOKIE_NAME", "DINCHY_AUTH_SSO_STATE_COOKIE_NAME",
		"DINCHY_AUTH_SESSION_IDLE_TIMEOUT", "DINCHY_AUTH_SESSION_MAX_LIFETIME",
		"DINCHY_AUTH_SSO_STATE_LIFETIME", "DINCHY_AUTH_PASSWORD_RESET_LIFETIME",
		"DINCHY_AUTH_TOTP_ISSUER", "DINCHY_AUTH_DEFAULT_ORGANIZATION_NAME",
		"DINCHY_AUTH_DEFAULT_ORGANIZATION_SLUG",
		"DINCHY_EVENT_BUS_STREAM_NAME", "DINCHY_EVENT_BUS_CONSUMER_GROUP_PREFIX",
		"DINCHY_EVENT_BUS_CONSUMER_NAME", "DINCHY_EVENT_BUS_BATCH_SIZE",
		"DINCHY_EVENT_BUS_RETENTION_WINDOW", "DINCHY_EVENT_BUS_CLAIM_MIN_IDLE",
		"DINCHY_EVENT_BUS_READ_BLOCK", "DINCHY_EVENT_BUS_WORKER_INTERVAL",
		"DINCHY_REDIS_ADDR", "DINCHY_REDIS_USERNAME",
		"DINCHY_REDIS_PASSWORD", "DINCHY_REDIS_DATABASE", "DINCHY_REDIS_KEY_PREFIX",
		"DINCHY_SMTP_HOST", "DINCHY_SMTP_PORT", "DINCHY_SMTP_USERNAME", "DINCHY_SMTP_PASSWORD", "DINCHY_SMTP_FROM",
		"DINCHY_REDIS_TLS", "DINCHY_PUBLIC_BASE_URL",
		"DINCHY_CADDY_ENABLED", "DINCHY_CADDY_ADMIN_ENDPOINT", "DINCHY_CADDY_ADMIN_TIMEOUT",
		"DINCHY_CADDY_EDGE_SERVER", "DINCHY_CADDY_TENANT", "DINCHY_CADDY_PANEL_UPSTREAM",
		"DINCHY_CADDY_PANEL_HOST", "DINCHY_CADDY_TLS_ISSUER",
		"DINCHY_CADDY_ACME_EMAIL", "DINCHY_CADDY_ACME_CA",
		"DINCHY_CADDY_HSTS_MAX_AGE", "DINCHY_CADDY_HSTS_INCLUDE_SUBDOMAINS",
		"DINCHY_ENV_FILE",
	} {
		t.Setenv(key, "")
	}
}
