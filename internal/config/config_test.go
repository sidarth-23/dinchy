package config_test

import (
	"testing"

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
	assert.Equal(t, "./dinchy.db", cfg.DBPath)
	assert.Equal(t, "http://127.0.0.1:5173", cfg.DevProxyURL)
	assert.False(t, cfg.DevMode)
	assert.False(t, cfg.RequireHTTPSForAuth)
}

func TestLoad_AllOverrides(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ADDR", ":9999")
	t.Setenv("DINCHY_INTERNAL_ADDR", ":8888")
	t.Setenv("DINCHY_DB_PATH", "/tmp/test.db")
	t.Setenv("DINCHY_DEV", "true")
	t.Setenv("DINCHY_DEV_PROXY_URL", "http://localhost:3000")
	t.Setenv("DINCHY_REQUIRE_HTTPS_FOR_AUTH", "1")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, ":9999", cfg.Addr)
	assert.Equal(t, ":8888", cfg.InternalAddr)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
	assert.True(t, cfg.DevMode)
	assert.Equal(t, "http://localhost:3000", cfg.DevProxyURL)
	assert.True(t, cfg.RequireHTTPSForAuth)
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

func TestLoad_MissingExplicitEnvFile_Fails(t *testing.T) {
	clearDinchyEnv(t)
	t.Setenv("DINCHY_ENV_FILE", "/tmp/definitely-does-not-exist-dinchy.env")

	_, err := config.Load()
	require.Error(t, err, "explicit env file that doesn't exist should fail")
}

// clearDinchyEnv clears all DINCHY_ env vars so tests start from a clean baseline.
func clearDinchyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DINCHY_ADDR", "DINCHY_INTERNAL_ADDR", "DINCHY_DB_PATH",
		"DINCHY_DEV", "DINCHY_DEV_PROXY_URL", "DINCHY_REQUIRE_HTTPS_FOR_AUTH",
		"DINCHY_ENV_FILE",
	} {
		t.Setenv(key, "")
	}
}
