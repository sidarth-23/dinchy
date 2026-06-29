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
	assert.Equal(t, ":8080", cfg.Addr)
	assert.Equal(t, ":9090", cfg.InternalAddr)
	assert.Equal(t, "sqlite", cfg.DBBackend)
	assert.Equal(t, "./dinchy.db", cfg.DBPath)
	assert.Equal(t, "http://127.0.0.1:5173", cfg.DevProxyURL)
	assert.False(t, cfg.DevMode)
	assert.False(t, cfg.RequireHTTPSForAuth)
	assert.Equal(t, "dinchy_session", cfg.Auth.SessionCookieName)
	assert.Equal(t, "dinchy_sso_state", cfg.Auth.SSOStateCookieName)
	assert.Equal(t, 30*time.Minute, cfg.Auth.SessionIdleTimeout)
	assert.Equal(t, 7*24*time.Hour, cfg.Auth.SessionMaxLifetime)
	assert.Equal(t, time.Hour, cfg.Auth.PasswordResetLifetime)
	assert.Empty(t, cfg.SSOProviders)
	assert.False(t, cfg.SMTP.Enabled())
}

func TestLoad_AllOverrides(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ADDR", ":9999")
	t.Setenv("DINCHY_INTERNAL_ADDR", ":8888")
	t.Setenv("DINCHY_DB_BACKEND", "sqlite")
	t.Setenv("DINCHY_DB_PATH", "/tmp/test.db")
	t.Setenv("DINCHY_DEV", "true")
	t.Setenv("DINCHY_DEV_PROXY_URL", "http://localhost:3000")
	t.Setenv("DINCHY_REQUIRE_HTTPS_FOR_AUTH", "1")
	t.Setenv("DINCHY_GITHUB_CLIENT_ID", "client")
	t.Setenv("DINCHY_GITHUB_CLIENT_SECRET", "secret")
	t.Setenv("DINCHY_GITHUB_CALLBACK_URL", "https://app.example.com/api/auth/sso/github/callback")
	t.Setenv("DINCHY_AUTH_SESSION_COOKIE_NAME", "custom_session")
	t.Setenv("DINCHY_AUTH_SESSION_IDLE_TIMEOUT", "45m")
	t.Setenv("DINCHY_SMTP_HOST", "smtp.example.com")
	t.Setenv("DINCHY_SMTP_FROM", "dinchy@example.com")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, ":9999", cfg.Addr)
	assert.Equal(t, ":8888", cfg.InternalAddr)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
	assert.True(t, cfg.DevMode)
	assert.Equal(t, "http://localhost:3000", cfg.DevProxyURL)
	assert.True(t, cfg.RequireHTTPSForAuth)
	require.Len(t, cfg.SSOProviders, 1)
	assert.Equal(t, config.SSOProviderGitHub, cfg.SSOProviders[0].ID)
	assert.Equal(t, "custom_session", cfg.Auth.SessionCookieName)
	assert.Equal(t, 45*time.Minute, cfg.Auth.SessionIdleTimeout)
	assert.True(t, cfg.SMTP.Enabled())
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

func TestLoad_MissingExplicitEnvFile_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ENV_FILE", "/tmp/definitely-does-not-exist-dinchy.env")

	_, err := config.Load()
	require.Error(t, err, "explicit env file that doesn't exist should fail")
}

func TestLoad_PostgresBackendRequiresDSN(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_DB_BACKEND", "postgres")

	_, err := config.Load()
	require.Error(t, err)
}

// clearDinchyEnv clears all DINCHY_ env vars so tests start from a clean baseline.
func clearDinchyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DINCHY_ADDR", "DINCHY_INTERNAL_ADDR", "DINCHY_DB_PATH",
		"DINCHY_DB_BACKEND", "DINCHY_POSTGRES_DSN",
		"DINCHY_DEV", "DINCHY_DEV_PROXY_URL", "DINCHY_REQUIRE_HTTPS_FOR_AUTH",
		"DINCHY_GOOGLE_CLIENT_ID", "DINCHY_GOOGLE_CLIENT_SECRET", "DINCHY_GOOGLE_CALLBACK_URL",
		"DINCHY_GITHUB_CLIENT_ID", "DINCHY_GITHUB_CLIENT_SECRET", "DINCHY_GITHUB_CALLBACK_URL",
		"DINCHY_MICROSOFT_CLIENT_ID", "DINCHY_MICROSOFT_CLIENT_SECRET", "DINCHY_MICROSOFT_CALLBACK_URL",
		"DINCHY_GITLAB_CLIENT_ID", "DINCHY_GITLAB_CLIENT_SECRET", "DINCHY_GITLAB_CALLBACK_URL",
		"DINCHY_AUTH_SESSION_COOKIE_NAME", "DINCHY_AUTH_SSO_STATE_COOKIE_NAME",
		"DINCHY_AUTH_SESSION_IDLE_TIMEOUT", "DINCHY_AUTH_SESSION_MAX_LIFETIME",
		"DINCHY_AUTH_SSO_STATE_LIFETIME", "DINCHY_AUTH_PASSWORD_RESET_LIFETIME",
		"DINCHY_AUTH_TOTP_ISSUER", "DINCHY_AUTH_DEFAULT_ORGANISATION_NAME",
		"DINCHY_AUTH_DEFAULT_ORGANISATION_SLUG",
		"DINCHY_SMTP_HOST", "DINCHY_SMTP_PORT", "DINCHY_SMTP_USERNAME", "DINCHY_SMTP_PASSWORD", "DINCHY_SMTP_FROM",
		"DINCHY_ENV_FILE",
	} {
		t.Setenv(key, "")
	}
}
