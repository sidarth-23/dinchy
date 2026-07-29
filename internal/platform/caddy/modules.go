package caddy

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// Module is one Caddy module compiled into the running binary.
type Module struct {
	// ID is the module's namespace-qualified identifier, such as
	// "dns.providers.cloudflare".
	ID string `json:"name"`
	// Package is the Go package the module came from, empty for standard modules.
	Package string `json:"package,omitempty"`
	// Version is the module's version as reported by Caddy.
	Version string `json:"version,omitempty"`
}

// ModuleSet is the set of modules a Caddy binary provides. A zero ModuleSet reports
// nothing as available and disables availability checking, which is how an
// unreadable Caddy binary degrades instead of blocking startup.
type ModuleSet struct {
	byID map[string]Module
}

// NewModuleSet builds a ModuleSet from the modules Caddy reported.
func NewModuleSet(modules []Module) ModuleSet {
	byID := make(map[string]Module, len(modules))
	for _, module := range modules {
		byID[module.ID] = module
	}
	return ModuleSet{byID: byID}
}

// Known reports whether the module list was read successfully. When false, no
// availability check is performed and Caddy's own rejection is the backstop.
func (s ModuleSet) Known() bool { return len(s.byID) > 0 }

// Has reports whether the given module identifier is compiled in. It always reports
// true for an unknown module set so a failed introspection never blocks a valid route.
func (s ModuleSet) Has(id string) bool {
	if !s.Known() {
		return true
	}
	_, ok := s.byID[id]
	return ok
}

// Modules returns the compiled-in modules ordered by identifier.
func (s ModuleSet) Modules() []Module {
	modules := make([]Module, 0, len(s.byID))
	for _, module := range s.byID {
		modules = append(modules, module)
	}
	slices.SortFunc(modules, func(a, b Module) int { return strings.Compare(a.ID, b.ID) })
	return modules
}

// DNSProviders returns the compiled-in DNS provider module identifiers, for offering
// the operator only the providers their Caddy build can actually use.
func (s ModuleSet) DNSProviders() []string {
	var providers []string
	for _, module := range s.Modules() {
		if strings.HasPrefix(module.ID, "dns.providers.") {
			providers = append(providers, module.ID)
		}
	}
	return providers
}

// ReadModuleSet runs the Caddy binary to discover which modules it provides.
//
// The binary is asked directly rather than reading the plugin manifest, because the
// manifest records what was requested while only the binary knows what actually
// registered — an xcaddy build can succeed with a module that registers nothing.
func ReadModuleSet(ctx context.Context, binary string) (ModuleSet, error) {
	output, err := exec.CommandContext(ctx, binary, "list-modules", "--json").Output()
	if err != nil {
		return ModuleSet{}, apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyListModules),
			apperrors.WithPath(apperrors.Path(binary)),
			apperrors.WithCause(fmt.Errorf("run %q list-modules: %w", binary, err)),
		)
	}
	var modules []Module
	if err := json.Unmarshal(output, &modules); err != nil {
		return ModuleSet{}, apperrors.Internal(
			i18n.Msg(i18n.CodeDiagnosticsCaddyListModules),
			apperrors.WithPath(apperrors.Path(binary)),
			apperrors.WithCause(fmt.Errorf("decode %q list-modules output: %w", binary, err)),
		)
	}
	return NewModuleSet(modules), nil
}

// checkModuleAvailability rejects a route needing a Caddy module that is not compiled
// in. Catching it here names the missing plugin, instead of letting the operator
// discover it minutes later as an unexplained certificate issuance failure.
func checkModuleAvailability(modules ModuleSet, route Route) error {
	if route.DNSProviderModule == "" || modules.Has(route.DNSProviderModule) {
		return nil
	}
	return apperrors.UnprocessableEntity(
		i18n.Msg(i18n.CodePlatformRoutingPluginMissing),
		apperrors.WithHostname(apperrors.Hostname(route.Host)),
		apperrors.WithModule(apperrors.Module(route.DNSProviderModule)),
		apperrors.WithOwner(apperrors.Owner(route.Owner)),
		apperrors.WithCause(fmt.Errorf("module %q is not compiled into this Caddy build", route.DNSProviderModule)),
	)
}
