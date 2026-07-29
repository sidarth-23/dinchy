package caddy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// Reconciler keeps Caddy's configuration in step with the routes its sources report.
type Reconciler struct {
	cfg     config.CaddyConfig
	admin   AdminClient
	clock   clock.Clock
	modules ModuleSet

	mu      sync.Mutex
	sources []RouteSource
	status  Status
}

// NewReconciler creates a Reconciler for the given configuration and admin client.
// The module set may be zero, which disables plugin availability checking.
func NewReconciler(cfg config.CaddyConfig, admin AdminClient, clk clock.Clock, modules ModuleSet) (*Reconciler, error) {
	if admin == nil {
		return nil, apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyReconcile),
			apperrors.WithCause(fmt.Errorf("admin client is required")),
		)
	}
	if clk == nil {
		clk = clock.System{}
	}
	return &Reconciler{cfg: cfg, admin: admin, clock: clk, modules: modules}, nil
}

// Register adds a RouteSource whose routes are pulled on every reconcile.
func (r *Reconciler) Register(sources ...RouteSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, source := range sources {
		if source != nil {
			r.sources = append(r.sources, source)
		}
	}
}

// Modules returns the Caddy module set this installation provides.
func (r *Reconciler) Modules() ModuleSet { return r.modules }

// Status returns a snapshot of reconcile health.
func (r *Reconciler) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// ReconcileAll rebuilds the whole configuration from every source and loads it.
//
// This is the startup path. It is the only place the entire configuration is replaced,
// because a full load closes active streaming connections — acceptable at boot, when
// there are none, and avoided afterwards via ApplyRoute and RemoveRoute.
func (r *Reconciler) ReconcileAll(ctx context.Context) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cfg.Enabled {
		return Result{}, nil
	}

	routes, err := r.collectLocked(ctx)
	if err != nil {
		r.recordFailureLocked(err)
		return Result{}, err
	}
	for i := range routes {
		if err := checkModuleAvailability(r.modules, routes[i]); err != nil {
			r.recordFailureLocked(err)
			return Result{}, err
		}
	}
	built, err := BuildConfig(r.cfg, routes)
	if err != nil {
		r.recordFailureLocked(err)
		return Result{}, err
	}
	if err := r.admin.LoadConfig(ctx, built); err != nil {
		r.recordFailureLocked(err)
		return Result{}, err
	}

	r.recordSuccessLocked(len(routes))
	return Result{RouteCount: len(routes), Reloaded: true}, nil
}

// ApplyRoute creates or replaces one route without touching the rest of the
// configuration, so connections served by unrelated routes — a web terminal, a log
// stream — survive the change.
func (r *Reconciler) ApplyRoute(ctx context.Context, route Route) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cfg.Enabled {
		return nil
	}

	resolved := route.Resolve()
	if err := resolved.Validate(); err != nil {
		return err
	}
	if err := validatePanelReservation(r.cfg, resolved); err != nil {
		return err
	}
	if err := checkModuleAvailability(r.modules, resolved); err != nil {
		return err
	}
	// Validate against the whole desired set so a new route cannot collide with, or be
	// shadowed by, one that already exists.
	existing, err := r.collectLocked(ctx)
	if err != nil {
		return err
	}
	if _, err := BuildConfig(r.cfg, upsert(existing, resolved)); err != nil {
		return err
	}
	if err := r.admin.PutRoute(ctx, buildServerRoute(r.cfg, resolved)); err != nil {
		return err
	}
	return nil
}

// RemoveRoute deletes one route by identity, leaving the rest of the configuration and
// its connections in place.
func (r *Reconciler) RemoveRoute(ctx context.Context, route Route) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cfg.Enabled {
		return nil
	}
	return r.admin.DeleteRoute(ctx, RouteID(route.Resolve()))
}

// collectLocked pulls the routes from every registered source. The caller holds r.mu.
func (r *Reconciler) collectLocked(ctx context.Context) ([]Route, error) {
	var routes []Route
	for _, source := range r.sources {
		sourceRoutes, err := source.Routes(ctx)
		if err != nil {
			return nil, annotateSourceFailure(source.Name(), err)
		}
		for i := range sourceRoutes {
			if sourceRoutes[i].Owner == "" {
				sourceRoutes[i].Owner = source.Name()
			}
			routes = append(routes, sourceRoutes[i])
		}
	}
	return routes, nil
}

// annotateSourceFailure attributes a source's failure to that source. A structured
// error keeps its own code and metadata so the owning feature's message survives; an
// unstructured one is converted here, at the first boundary that can name the source.
func annotateSourceFailure(name string, err error) error {
	if _, ok := errors.AsType[*apperrors.AppError](err); ok {
		return apperrors.Annotate(err, apperrors.WithOwner(apperrors.Owner(name)))
	}
	return apperrors.Internal(
		i18n.Msg(i18n.CodeDiagnosticsCaddyCollectRoutes),
		apperrors.WithOwner(apperrors.Owner(name)),
		apperrors.WithCause(fmt.Errorf("collect routes from source %q: %w", name, err)),
	)
}

func (r *Reconciler) recordSuccessLocked(routeCount int) {
	now := r.clock.Now()
	r.status = Status{
		LastAttemptAt: now,
		LastSuccessAt: now,
		RouteCount:    routeCount,
	}
}

func (r *Reconciler) recordFailureLocked(err error) {
	r.status.LastAttemptAt = r.clock.Now()
	r.status.LastError = err.Error()
	r.status.Degraded = true
}

// upsert replaces the route with the same identity, or appends it.
func upsert(routes []Route, route Route) []Route {
	id := RouteID(route)
	out := make([]Route, 0, len(routes)+1)
	replaced := false
	for i := range routes {
		if RouteID(routes[i].Resolve()) == id {
			out = append(out, route)
			replaced = true
			continue
		}
		out = append(out, routes[i])
	}
	if !replaced {
		out = append(out, route)
	}
	return out
}
