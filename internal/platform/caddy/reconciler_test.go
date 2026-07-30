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

// spyAdmin records what the reconciler pushed to the edge, in the order it pushed it.
type spyAdmin struct {
	mu        sync.Mutex
	applied   []string
	routes    int
	policies  int
	routeErr  error
	policyErr error
	pingErr   error
	hosts     []string
	server    string
}

func (s *spyAdmin) ApplyRoute(_ context.Context, edgeServerName string, route caddy.ServerRoute) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applied = append(s.applied, "route")
	s.routes++
	if s.routeErr != nil {
		return s.routeErr
	}
	s.server = edgeServerName
	s.hosts = nil
	for _, handler := range route.Handle {
		for _, nested := range handler.Routes {
			s.hosts = append(s.hosts, nested.Match[0].Host[0])
		}
	}
	return nil
}

func (s *spyAdmin) ApplyTLSPolicy(_ context.Context, _ caddy.AutomationPolicy) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applied = append(s.applied, "policy")
	s.policies++
	return s.policyErr
}

func (s *spyAdmin) Ping(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingErr
}

// writeCount is how many configuration writes reached the edge, of either kind.
func (s *spyAdmin) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routes + s.policies
}

func (s *spyAdmin) order() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.applied...)
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

func TestReconcileAll_AppliesTheRouteAndThePolicy(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	result, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	assert.True(t, result.Applied)
	assert.Equal(t, 1, result.RouteCount)
	assert.Equal(t, 2, admin.writeCount(), "one route object and one policy object")
	assert.Equal(t, cfg.EdgeServerName, admin.server)
}

// TestReconcileAll_AppliesThePolicyBeforeTheRoute pins the order, because the two half-applied
// states are not equally bad. A route without its policy leaves the host answering with no
// certificate, so the handshake fails; a policy without its route provisions a certificate nothing
// uses yet and breaks nothing.
func TestReconcileAll_AppliesThePolicyBeforeTheRoute(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	_, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"policy", "route"}, admin.order())
}

// TestReconcileAll_PolicyFailureStopsBeforeTheRoute follows from the ordering above: publishing a
// route whose certificate could not be arranged would break the host outright.
func TestReconcileAll_PolicyFailureStopsBeforeTheRoute(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{policyErr: apperrors.Internal(i18n.Msg(i18n.CodePlatformRoutingApplyFailed))}
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg))

	_, err := reconciler.ReconcileAll(context.Background())

	assertCode(t, err, i18n.CodePlatformRoutingApplyFailed, http.StatusInternalServerError)
	assert.Equal(t, []string{"policy"}, admin.order(), "the route must not be published")
}

func TestReconcileAll_NoRoutesPushesNothing(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	reconciler := newReconciler(t, cfg, admin)

	result, err := reconciler.ReconcileAll(context.Background())
	require.NoError(t, err)

	assert.False(t, result.Applied)
	assert.Zero(t, admin.writeCount())
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
	assert.Zero(t, admin.writeCount(), "a rejected route set must not reach Caddy")
}

func TestReconcileAll_SourceFailureAbortsBeforeTouchingCaddy(t *testing.T) {
	cfg := productionConfig()
	admin := &spyAdmin{}
	sourceErr := apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsCaddyCollectRoutes))
	reconciler := newReconciler(t, cfg, admin, panelSource(cfg), failingSource{err: sourceErr})

	_, err := reconciler.ReconcileAll(context.Background())
	require.Error(t, err)

	assert.Zero(t, admin.writeCount())

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

	assert.False(t, result.Applied)
	assert.Zero(t, admin.writeCount())
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

	assert.Equal(t, 16, admin.writeCount(), "eight reconciles, two writes each")
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
	assert.Contains(t, admin.hosts, "app.example.com")
}
