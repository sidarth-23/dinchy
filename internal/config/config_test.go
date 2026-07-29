package config_test

import (
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
	assert.Equal(t, "http://127.0.0.1:5173", cfg.DevProxyURL)
	assert.False(t, cfg.DevMode)
	assert.True(t, cfg.Caddy.Enabled)
	assert.Equal(t, "127.0.0.1:2019", cfg.Caddy.AdminEndpoint)
	assert.Equal(t, uint16(443), cfg.Caddy.HTTPSPort)
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
	t.Setenv("DINCHY_DEV_PROXY_URL", "http://localhost:3000")
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
	t.Setenv("DINCHY_CADDY_HTTPS_PORT", "8443")
	t.Setenv("DINCHY_CADDY_PANEL_HOST", "Panel.Example.COM")
	t.Setenv("DINCHY_CADDY_ACME_EMAIL", "ops@example.com")
	t.Setenv("DINCHY_CADDY_STORAGE_PATH", "/srv/caddy-data")
	t.Setenv("DINCHY_CADDY_HSTS_MAX_AGE", "48h")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9999", cfg.Addr)
	assert.Equal(t, "127.0.0.1:8888", cfg.InternalAddr)
	assert.Equal(t, "postgres://test:test@localhost:5432/dinchy?sslmode=disable", cfg.Database.PostgresDSN)
	assert.True(t, cfg.DevMode)
	assert.Equal(t, "http://localhost:3000", cfg.DevProxyURL)
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
	assert.Equal(t, uint16(8443), cfg.Caddy.HTTPSPort)
	assert.Equal(t, "panel.example.com", cfg.Caddy.PanelHost, "panel host is lowercased so route comparisons are case-insensitive")
	assert.Equal(t, "ops@example.com", cfg.Caddy.ACMEEmail)
	assert.Equal(t, "/srv/caddy-data", cfg.Caddy.StoragePath)
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

func TestLoad_InvalidDevProxyURL_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_DEV_PROXY_URL", "not-a-url")

	_, err := config.Load()
	require.Error(t, err, "invalid DevProxyURL should fail validation")
}

func TestLoad_DevMode_DefaultProxyURLPassesValidation(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_DEV", "true")
	// DevProxyURL intentionally not set — default "http://127.0.0.1:5173" must satisfy required_if.

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.True(t, cfg.DevMode)
	assert.Equal(t, "http://127.0.0.1:5173", cfg.DevProxyURL)
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

func TestLoad_RejectsNonLoopbackAddr(t *testing.T) {
	// The app trusts the forwarded client address and carries no transport check of its
	// own, so restricting who can reach the listener is the boundary that replaces one.
	for _, addr := range []string{":8080", "0.0.0.0:8080", "10.0.0.5:8080", "[::]:8080"} {
		t.Run(addr, func(t *testing.T) {
			clearDinchyEnv(t)
			t.Setenv("DINCHY_ADDR", addr)

			_, err := config.Load()

			require.Error(t, err)
			assert.Contains(t, err.(interface{ Unwrap() error }).Unwrap().Error(), "DINCHY_ADDR",
				"the message must name the variable so the fix is obvious")
		})
	}
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

func TestFrontendUpstream(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_DEV_PROXY_URL", "http://127.0.0.1:5173")

	cfg, err := config.Load()
	require.NoError(t, err)

	// Caddy needs a dial address, not a URL, because it proxies to Vite itself.
	assert.Equal(t, "127.0.0.1:5173", cfg.FrontendUpstream())
}

func TestLoad_FrontendRootDefault(t *testing.T) {
	clearDinchyEnv(t)

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "web/dist", cfg.FrontendRoot)
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
		"DINCHY_ADDR", "DINCHY_INTERNAL_ADDR", "DINCHY_POSTGRES_DSN",
		"DINCHY_DEV", "DINCHY_DEV_PROXY_URL", "DINCHY_FRONTEND_ROOT",
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
		"DINCHY_CADDY_HTTPS_PORT",
		"DINCHY_CADDY_PANEL_HOST", "DINCHY_CADDY_TLS_ISSUER",
		"DINCHY_CADDY_ACME_EMAIL", "DINCHY_CADDY_ACME_CA", "DINCHY_CADDY_STORAGE_PATH",
		"DINCHY_CADDY_HSTS_MAX_AGE", "DINCHY_CADDY_HSTS_INCLUDE_SUBDOMAINS",
		"DINCHY_ENV_FILE",
	} {
		t.Setenv(key, "")
	}
}
