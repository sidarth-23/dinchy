package caddy

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// Reconciler keeps Caddy's configuration in step with the routes its sources report.
type Reconciler struct {
	cfg   config.CaddyConfig
	admin AdminClient

	mu      sync.Mutex
	sources []RouteSource
}

// NewReconciler creates a Reconciler for the given configuration and admin client.
func NewReconciler(cfg config.CaddyConfig, admin AdminClient) (*Reconciler, error) {
	if admin == nil {
		return nil, apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyReconcile),
			apperrors.WithCause(fmt.Errorf("admin client is required")),
		)
	}
	return &Reconciler{cfg: cfg, admin: admin}, nil
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

// Ping reports whether Caddy's admin API is reachable right now. Readiness asks this
// rather than remembering how the last push went: Dinchy pushes once at startup and never
// re-asserts, so a remembered outcome would go stale the moment anything changed.
func (r *Reconciler) Ping(ctx context.Context) error {
	return r.admin.Ping(ctx)
}

// ReconcileAll rebuilds this deployment's slice of the edge's configuration from every source and
// applies it, as two addressable objects. Every other object in the running configuration is left
// exactly as it was, which is what lets several applications share one edge.
//
// This runs once, at startup. Dinchy converges the edge on the routes it owns and then leaves it
// alone: an operator who changes the running configuration afterwards keeps that change, because a
// management plane that re-asserts on a timer is one an operator cannot work with.
//
// The policy is written before the route, because the two failure orders are not equally bad. A
// route without its policy leaves the host answering with no certificate, so the handshake fails; a
// policy without its route provisions a certificate nothing uses yet and breaks nothing.
func (r *Reconciler) ReconcileAll(ctx context.Context) (Result, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.cfg.Enabled {
		return Result{}, nil
	}

	routes, err := r.collectLocked(ctx)
	if err != nil {
		return Result{}, err
	}
	contribution, err := BuildContribution(r.cfg, routes)
	if err != nil {
		return Result{}, err
	}
	if contribution.Empty() {
		return Result{}, nil
	}

	if err := r.admin.ApplyTLSPolicy(ctx, contribution.Policy); err != nil {
		return Result{}, err
	}
	if err := r.admin.ApplyRoute(ctx, r.cfg.EdgeServerName, contribution.Route); err != nil {
		return Result{}, err
	}
	return Result{RouteCount: contribution.RouteCount, Applied: true}, nil
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
