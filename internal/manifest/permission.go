package manifest

import (
	"fmt"
	"os"
	"strings"
)

// PermissionCatalog is the combined access-policy manifest assembled from the
// permissions and roles source documents.
type PermissionCatalog struct {
	Modules []PermissionModule
	Roles   []PermissionRole
}

// PermissionModulesDocument is the root of the permissions source file.
type PermissionModulesDocument struct {
	Modules []PermissionModule `json:"modules"`
}

// PermissionRolesDocument is the root of the roles source file.
type PermissionRolesDocument struct {
	Roles []PermissionRole `json:"roles"`
}

// PermissionModule groups permission entries under a stable module ID.
type PermissionModule struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Entries     []PermissionEntry `json:"entries"`
}

// PermissionEntry describes a module resource action.
type PermissionEntry struct {
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description"`
	I18nCode    string `json:"i18n_code"`
}

// PermissionRole describes a built-in role and its permission grants.
type PermissionRole struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	I18nCode    string   `json:"i18n_code"`
	Permissions []string `json:"permissions"`
}

// DecodePermissionModules strictly decodes the permissions source document.
func DecodePermissionModules(raw []byte) ([]PermissionModule, error) {
	var document PermissionModulesDocument
	if err := decodeStrict(raw, &document); err != nil {
		return nil, err
	}
	return document.Modules, nil
}

// DecodePermissionRoles strictly decodes the roles source document.
func DecodePermissionRoles(raw []byte) ([]PermissionRole, error) {
	var document PermissionRolesDocument
	if err := decodeStrict(raw, &document); err != nil {
		return nil, err
	}
	return document.Roles, nil
}

// LoadPermissionCatalog reads the permissions and roles source files and returns
// the combined catalog. It decodes but does not validate; callers validate the
// result with ValidatePermissionCatalog.
func LoadPermissionCatalog(permissionsPath, rolesPath string) (PermissionCatalog, error) {
	permissionsRaw, err := os.ReadFile(permissionsPath)
	if err != nil {
		return PermissionCatalog{}, fmt.Errorf("read permissions %q: %w", permissionsPath, err)
	}
	modules, err := DecodePermissionModules(permissionsRaw)
	if err != nil {
		return PermissionCatalog{}, fmt.Errorf("decode permissions %q: %w", permissionsPath, err)
	}
	rolesRaw, err := os.ReadFile(rolesPath)
	if err != nil {
		return PermissionCatalog{}, fmt.Errorf("read roles %q: %w", rolesPath, err)
	}
	roles, err := DecodePermissionRoles(rolesRaw)
	if err != nil {
		return PermissionCatalog{}, fmt.Errorf("decode roles %q: %w", rolesPath, err)
	}
	return PermissionCatalog{Modules: modules, Roles: roles}, nil
}

// ValidatePermissionCatalog validates IDs, entries, role grants, and i18n references.
func ValidatePermissionCatalog(catalog PermissionCatalog, i18nCatalog I18nCatalog) error {
	if len(catalog.Modules) == 0 || len(catalog.Roles) == 0 {
		return fmt.Errorf("permission catalog must define modules and roles")
	}
	i18nCodes := map[string]struct{}{}
	collectI18nCodes(i18nCatalog.Modules, nil, i18nCodes)
	permissions := map[string]struct{}{}
	modules := map[string]struct{}{}
	for _, module := range catalog.Modules {
		if !validPermissionSegment(module.ID) || module.Description == "" {
			return fmt.Errorf("invalid permission module %q", module.ID)
		}
		if _, ok := modules[module.ID]; ok {
			return fmt.Errorf("duplicate permission module %q", module.ID)
		}
		modules[module.ID] = struct{}{}
		for _, entry := range module.Entries {
			key := PermissionKey(module.ID, entry.Resource, entry.Action)
			if !validPermissionSegment(entry.Resource) || !validPermissionSegment(entry.Action) || entry.Description == "" || entry.I18nCode == "" {
				return fmt.Errorf("invalid permission %q", key)
			}
			if _, ok := permissions[key]; ok {
				return fmt.Errorf("duplicate permission %q", key)
			}
			permissions[key] = struct{}{}
			if _, ok := i18nCodes[entry.I18nCode]; !ok {
				return fmt.Errorf("permission %q references unknown i18n code %q", key, entry.I18nCode)
			}
		}
	}
	roles := map[string]struct{}{}
	for _, role := range catalog.Roles {
		if !validPermissionSegment(role.ID) || role.Description == "" || role.I18nCode == "" {
			return fmt.Errorf("invalid role %q", role.ID)
		}
		if _, ok := roles[role.ID]; ok {
			return fmt.Errorf("duplicate role %q", role.ID)
		}
		roles[role.ID] = struct{}{}
		if _, ok := i18nCodes[role.I18nCode]; !ok {
			return fmt.Errorf("role %q references unknown i18n code %q", role.ID, role.I18nCode)
		}
		for _, value := range role.Permissions {
			if _, ok := permissions[value]; !ok {
				return fmt.Errorf("role %q references unknown permission %q", role.ID, value)
			}
		}
	}
	return nil
}

// PermissionKey builds a stable module resource action key.
func PermissionKey(module, resource, action string) string {
	return strings.Join([]string{module, resource, action}, ".")
}

func validPermissionSegment(value string) bool {
	return value != "" && !strings.ContainsAny(value, ". \t\n")
}

func collectI18nCodes(modules []I18nModule, path []string, codes map[string]struct{}) {
	for _, module := range modules {
		current := append(append([]string{}, path...), module.Name)
		for _, message := range module.Messages {
			codes[I18nCodeFor(current, message.Name)] = struct{}{}
		}
		collectI18nCodes(module.Modules, current, codes)
	}
}
