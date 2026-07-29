package caddy_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// writeFakeCaddy writes an executable stub that answers `list-modules --json`.
func writeFakeCaddy(t *testing.T, stdout string, exitCode int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "caddy")
	script := "#!/bin/sh\ncat <<'JSON'\n" + stdout + "\nJSON\nexit " + strconv.Itoa(exitCode) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	return path
}

func TestReadModuleSet_ParsesCaddyOutput(t *testing.T) {
	binary := writeFakeCaddy(t, `[
	  {"name":"dns.providers.cloudflare","package":"github.com/caddy-dns/cloudflare","version":"v0.2.1"},
	  {"name":"http.handlers.reverse_proxy"}
	]`, 0)

	modules, err := caddy.ReadModuleSet(context.Background(), binary)
	require.NoError(t, err)

	assert.True(t, modules.Known())
	assert.True(t, modules.Has("dns.providers.cloudflare"))
	assert.False(t, modules.Has("dns.providers.route53"))
	assert.Equal(t, []string{"dns.providers.cloudflare"}, modules.DNSProviders())

	listed := modules.Modules()
	require.Len(t, listed, 2)
	assert.Equal(t, "dns.providers.cloudflare", listed[0].ID, "modules are ordered by identifier")
	assert.Equal(t, "v0.2.1", listed[0].Version)
}

func TestReadModuleSet_MissingBinaryReportsListModulesFailure(t *testing.T) {
	err := func() error {
		_, err := caddy.ReadModuleSet(context.Background(), filepath.Join(t.TempDir(), "absent"))
		return err
	}()

	assertCode(t, err, i18n.CodeDiagnosticsCaddyListModules, http.StatusInternalServerError)
}

func TestReadModuleSet_UnparseableOutputReportsListModulesFailure(t *testing.T) {
	binary := writeFakeCaddy(t, `not json at all`, 0)

	_, err := caddy.ReadModuleSet(context.Background(), binary)

	assertCode(t, err, i18n.CodeDiagnosticsCaddyListModules, http.StatusInternalServerError)
}

// TestModuleSet_UnknownSetAllowsEverything pins the degradation contract: when the
// module list could not be read, availability checking is skipped rather than
// rejecting every route that names a plugin.
func TestModuleSet_UnknownSetAllowsEverything(t *testing.T) {
	var unknown caddy.ModuleSet

	assert.False(t, unknown.Known())
	assert.True(t, unknown.Has("dns.providers.cloudflare"))
	assert.Empty(t, unknown.Modules())
	assert.Empty(t, unknown.DNSProviders())
}
