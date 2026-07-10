package manifest

import (
	"fmt"
	"strings"
)

// PermissionCatalog is the root of the access-policy manifest.
type PermissionCatalog struct {
	Modules []PermissionModule `json:"modules"`
	Roles   []PermissionRole   `json:"roles"`
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

// DecodePermissionCatalog strictly decodes a permission manifest.
func DecodePermissionCatalog(raw []byte) (PermissionCatalog, error) {
	var catalog PermissionCatalog
	if err := decodeStrict(raw, &catalog); err != nil {
		return PermissionCatalog{}, err
	}
	return catalog, nil
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
