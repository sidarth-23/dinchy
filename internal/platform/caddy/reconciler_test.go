package caddy_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// spyAdmin records what the reconciler pushed to Caddy.
type spyAdmin struct {
	mu         sync.Mutex
	loads      int
	loadErr    error
	pingErr    error
	loadedHost []string
}

func (s *spyAdmin) LoadConfig(_ context.Context, cfg caddy.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	if s.loadErr != nil {
		return s.loadErr
	}
	s.loadedHost = nil
	if cfg.Apps != nil && cfg.Apps.HTTP != nil {
		for _, route := range cfg.Apps.HTTP.Servers[caddy.ServerName].Routes {
			s.loadedHost = append(s.loadedHost, route.Match[0].Host[0])
		}
	}
	return nil
}

func (s *spyAdmin) Ping(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingErr
}

func (s *spyAdmin) loadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loads
}

// failingSource is a RouteSource that always fails.
type failingSource struct{ err error }

func (f failingSource) Name() string { return "failing" }

func (f failingSource) Routes(context.Context) ([]caddy.Route, error) { return nil, f.err }

func newReconciler(t *testing.T, cfg config.CaddyConfig, admin caddy.AdminClient, sources ...caddy.RouteSource) *caddy.Reconciler {
	t.Helper()
	reconciler, err := caddy.NewReconciler(cfg, admin)
	require.NoError(t, err)
	reconciler.Register(sources...)
	return reconciler
}

func panelSource(cfg config.CaddyConfig) caddy.RouteSource {
	return caddy.NewStaticSource(caddy.PanelOwner, panelRoute(cfg.PanelHost, "127.0.0.1:8080"))
}

func TestReconcileAll_LoadsTheWholeConfigurationOnce(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	result, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	assert.True(t, result.Reloaded)
	assert.Equal(t, 1, result.RouteCount)
	assert.Equal(t, 1, admin.loadCount())
}

func TestReconcileAll_RejectsConflictingRoutesWithoutCallingCaddy(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	first := caddy.NewStaticSource("deployments", caddy.Route{
		Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:32769",
	})
	second := caddy.NewStaticSource("imports", caddy.Route{
		Owner: "imports", Host: "app.example.com", Upstream: "127.0.0.1:32770",
	})
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg), first, second)

	_, err := reconciler.ReconcileAll(context.Background())

	assertCode(t, err, i18n.CodePlatformRoutingHostConflict, http.StatusConflict)
	assert.Zero(t, admin.loadCount(), "a rejected route set must not reach Caddy")
}

func TestReconcileAll_SourceFailureAbortsBeforeTouchingCaddy(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	sourceErr := apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsCaddyCollectRoutes))
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg), failingSource{err: sourceErr})

	_, err := reconciler.ReconcileAll(context.Background())
	require.Error(t, err)

	assert.Zero(t, admin.loadCount())

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "failing", appErr.Meta()[string(apperrors.MetaKeyOwner)], "the failure is attributed to its source")
}

func TestReconcileAll_UnstructuredSourceErrorBecomesStructured(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, failingSource{err: errors.New("database is down")})

	_, err := reconciler.ReconcileAll(context.Background())

	assertCode(t, err, i18n.CodeDiagnosticsCaddyCollectRoutes, http.StatusInternalServerError)
}

func TestPing_ReportsWhetherCaddyAnswersNow(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{pingErr: apperrors.Internal(i18n.Msg(i18n.CodePlatformRoutingUnavailable))}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	require.Error(t, reconciler.Ping(context.Background()))

	// Readiness reflects the proxy's current state, not how the startup push went, so a
	// Caddy that comes back is reported healthy with no reconcile in between.
	admin.mu.Lock()
	admin.pingErr = nil
	admin.mu.Unlock()

	require.NoError(t, reconciler.Ping(context.Background()))
}

func TestReconciler_DisabledDoesNothing(t *testing.T) {
	cfg := productionConfig()
	cfg.Enabled = false
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	result, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	assert.False(t, result.Reloaded)
	assert.Zero(t, admin.loadCount())
}

func TestReconciler_ConcurrentCallsSerialize(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = reconciler.ReconcileAll(context.Background())
		}()
	}
	wg.Wait()

	assert.Equal(t, 8, admin.loadCount())
}

func TestNewReconciler_RequiresAnAdminClient(t *testing.T) {
	_, err := caddy.NewReconciler(productionConfig(), nil)

	assertCode(t, err, i18n.CodeDiagnosticsCaddyReconcile, http.StatusInternalServerError)
}

func TestStaticSource_DefaultsOwnerToSourceName(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	// A route contributed without an Owner is attributed to the source that produced it.
	source := caddy.NewStaticSource("deployments", caddy.Route{Host: "app.example.com", Upstream: "127.0.0.1:32769"})
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg), source)

	_, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	admin.mu.Lock()
	defer admin.mu.Unlock()
	assert.Contains(t, admin.loadedHost, "app.example.com")
}
