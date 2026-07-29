package caddy_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/caddy"
)

// spyAdmin records which admin operations the reconciler performed, so a test can
// assert that a single route change did not replace the whole configuration.
type spyAdmin struct {
	mu           sync.Mutex
	loads        int
	putRouteIDs  []string
	deletedIDs   []string
	loadErr      error
	putRouteErr  error
	lastLoadedID []string
}

func (s *spyAdmin) LoadConfig(_ context.Context, cfg caddy.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loads++
	if s.loadErr != nil {
		return s.loadErr
	}
	s.lastLoadedID = nil
	if cfg.Apps != nil && cfg.Apps.HTTP != nil {
		for _, route := range cfg.Apps.HTTP.Servers[caddy.ServerName].Routes {
			s.lastLoadedID = append(s.lastLoadedID, route.ID)
		}
	}
	return nil
}

func (s *spyAdmin) PutRoute(_ context.Context, route caddy.ServerRoute) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putRouteErr != nil {
		return s.putRouteErr
	}
	s.putRouteIDs = append(s.putRouteIDs, route.ID)
	return nil
}

func (s *spyAdmin) DeleteRoute(_ context.Context, routeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletedIDs = append(s.deletedIDs, routeID)
	return nil
}

func (s *spyAdmin) Ping(context.Context) error { return nil }

func (s *spyAdmin) snapshot() (loads int, puts, deletes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loads, append([]string(nil), s.putRouteIDs...), append([]string(nil), s.deletedIDs...)
}

// failingSource is a RouteSource that always fails.
type failingSource struct{ err error }

func (f failingSource) Name() string { return "failing" }

func (f failingSource) Routes(context.Context) ([]caddy.Route, error) { return nil, f.err }

func newReconciler(t *testing.T, cfg config.CaddyConfig, admin caddy.AdminClient, sources ...caddy.RouteSource) *caddy.Reconciler {
	t.Helper()
	reconciler, err := caddy.NewReconciler(cfg, admin, clock.System{}, caddy.ModuleSet{})
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
	loads, puts, _ := admin.snapshot()
	assert.Equal(t, 1, loads)
	assert.Empty(t, puts)
}

// TestApplyRoute_DoesNotReplaceTheWholeConfiguration guards the property that made the
// admin API the right choice: replacing the whole configuration makes Caddy close
// active streaming connections, so adding one deployment must not do it.
func TestApplyRoute_DoesNotReplaceTheWholeConfiguration(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	require.NoError(t, reconciler.ApplyRoute(context.Background(), caddy.Route{
		Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:32769",
	}))

	loads, puts, _ := admin.snapshot()
	assert.Zero(t, loads, "a single route change must never reload the whole configuration")
	assert.Equal(t, []string{"dinchy-deployments-app.example.com-root"}, puts)
}

func TestRemoveRoute_DeletesByStableID(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	require.NoError(t, reconciler.RemoveRoute(context.Background(), caddy.Route{
		Owner: "deployments", Host: "App.Example.com", Upstream: "127.0.0.1:32769",
	}))

	loads, _, deletes := admin.snapshot()
	assert.Zero(t, loads)
	assert.Equal(t, []string{"dinchy-deployments-app.example.com-root"}, deletes)
}

func TestApplyRoute_RejectsConflictWithAnExistingRouteWithoutCallingCaddy(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	existing := caddy.NewStaticSource("deployments", caddy.Route{
		Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:32769",
	})
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg), existing)

	err := reconciler.ApplyRoute(context.Background(), caddy.Route{
		Owner: "imports", Host: "app.example.com", Upstream: "127.0.0.1:32770",
	})

	assertCode(t, err, i18n.CodePlatformRoutingHostConflict, http.StatusConflict)
	loads, puts, _ := admin.snapshot()
	assert.Zero(t, loads)
	assert.Empty(t, puts, "a rejected route must not reach Caddy")
}

func TestApplyRoute_RejectsClaimingThePanelHostWithoutCallingCaddy(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	err := reconciler.ApplyRoute(context.Background(), caddy.Route{
		Owner: "deployments", Host: cfg.PanelHost, Upstream: "127.0.0.1:32769",
	})

	assertCode(t, err, i18n.CodePlatformRoutingPanelHostReserved, http.StatusConflict)
	_, puts, _ := admin.snapshot()
	assert.Empty(t, puts)
}

func TestReconcileAll_SourceFailureAbortsBeforeTouchingCaddy(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	sourceErr := apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsCaddyCollectRoutes))
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg), failingSource{err: sourceErr})

	_, err := reconciler.ReconcileAll(context.Background())
	require.Error(t, err)

	loads, _, _ := admin.snapshot()
	assert.Zero(t, loads)

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

func TestStatus_ReportsDegradedAfterFailureAndRecovers(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{loadErr: apperrors.Internal(i18n.Msg(i18n.CodePlatformRoutingUnavailable))}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	_, err := reconciler.ReconcileAll(context.Background())
	require.Error(t, err)

	degraded := reconciler.Status()
	assert.True(t, degraded.Degraded)
	assert.NotEmpty(t, degraded.LastError)
	assert.False(t, degraded.LastAttemptAt.IsZero())
	assert.True(t, degraded.LastSuccessAt.IsZero())

	// The recurring job retries, so a Caddy that comes up later converges with no
	// operator action.
	admin.mu.Lock()
	admin.loadErr = nil
	admin.mu.Unlock()

	_, err = reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	healthy := reconciler.Status()
	assert.False(t, healthy.Degraded)
	assert.Empty(t, healthy.LastError)
	assert.Equal(t, 1, healthy.RouteCount)
}

func TestReconciler_DisabledDoesNothing(t *testing.T) {
	cfg := productionConfig()
	cfg.Enabled = false
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	result, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)
	require.NoError(t, reconciler.ApplyRoute(context.Background(), caddy.Route{
		Owner: "deployments", Host: "app.example.com", Upstream: "127.0.0.1:32769",
	}))

	assert.False(t, result.Reloaded)
	loads, puts, _ := admin.snapshot()
	assert.Zero(t, loads)
	assert.Empty(t, puts)
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

	loads, _, _ := admin.snapshot()
	assert.Equal(t, 8, loads)
	assert.False(t, reconciler.Status().Degraded)
}

func TestNewReconciler_RequiresAnAdminClient(t *testing.T) {
	_, err := caddy.NewReconciler(productionConfig(), nil, clock.System{}, caddy.ModuleSet{})

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
	assert.Contains(t, admin.lastLoadedID, "dinchy-deployments-app.example.com-root")
}

func TestReconcileAll_RejectsRouteNeedingAnUninstalledPlugin(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	modules := caddy.NewModuleSet([]caddy.Module{{ID: "dns.providers.route53"}})
	reconciler, err := caddy.NewReconciler(cfg, admin, clock.System{}, modules)
	require.NoError(t, err)
	reconciler.Register(panelSource(cfg), caddy.NewStaticSource("deployments", caddy.Route{
		Owner: "deployments", Host: "nat.example.com", Upstream: "127.0.0.1:32769",
		DNSProviderModule: "dns.providers.cloudflare",
	}))

	_, err = reconciler.ReconcileAll(context.Background())

	// Naming the missing plugin here beats an unexplained issuance failure later.
	assertCode(t, err, i18n.CodePlatformRoutingPluginMissing, http.StatusUnprocessableEntity)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "dns.providers.cloudflare", appErr.Meta()[string(apperrors.MetaKeyModule)])
	loads, _, _ := admin.snapshot()
	assert.Zero(t, loads)
}

func TestReconcileAll_AllowsRouteWhenPluginIsInstalled(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	modules := caddy.NewModuleSet([]caddy.Module{{ID: "dns.providers.cloudflare"}})
	reconciler, err := caddy.NewReconciler(cfg, admin, clock.System{}, modules)
	require.NoError(t, err)
	reconciler.Register(panelSource(cfg), caddy.NewStaticSource("deployments", caddy.Route{
		Owner: "deployments", Host: "nat.example.com", Upstream: "127.0.0.1:32769",
		DNSProviderModule: "dns.providers.cloudflare",
	}))

	_, err = reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)
}

// TestReconcileAll_UnknownModuleSetSkipsTheAvailabilityCheck pins the degradation
// contract: an unreadable Caddy binary must not block routing.
func TestReconcileAll_UnknownModuleSetSkipsTheAvailabilityCheck(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg), caddy.NewStaticSource("deployments", caddy.Route{
		Owner: "deployments", Host: "nat.example.com", Upstream: "127.0.0.1:32769",
		DNSProviderModule: "dns.providers.cloudflare",
	}))

	_, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err, "Caddy's own rejection is the backstop when the module set is unknown")
}

func TestReconciler_StatusIsASnapshot(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	before := reconciler.Status()
	_, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	assert.True(t, before.LastAttemptAt.IsZero(), "a snapshot taken earlier is not mutated by a later reconcile")
	assert.WithinDuration(t, time.Now(), reconciler.Status().LastSuccessAt, time.Minute)
}
