package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func i18nCatalogWithCodes(codes ...string) I18nCatalog {
	catalog := I18nCatalog{}
	for _, code := range codes {
		index := strings.LastIndex(code, ".")
		catalog.Modules = append(catalog.Modules, I18nModule{
			Name:     code[:index],
			Messages: []I18nMessage{{Name: code[index+1:]}},
		})
	}
	return catalog
}

func TestLoadPermissionCatalogCombinesFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	permissionsPath := filepath.Join(dir, "permissions.json")
	rolesPath := filepath.Join(dir, "roles.json")
	require.NoError(t, os.WriteFile(permissionsPath, []byte(
		`{"modules":[{"id":"audit","description":"Audit","entries":[{"resource":"logs","action":"read","description":"View logs","i18n_code":"perm.audit.logs.read"}]}]}`), 0o644))
	require.NoError(t, os.WriteFile(rolesPath, []byte(
		`{"roles":[{"id":"admin","description":"Admin","i18n_code":"role.admin","permissions":["audit.logs.read"]}]}`), 0o644))

	catalog, err := LoadPermissionCatalog(permissionsPath, rolesPath)
	require.NoError(t, err)
	require.Len(t, catalog.Modules, 1)
	require.Equal(t, "audit", catalog.Modules[0].ID)
	require.Len(t, catalog.Roles, 1)
	require.Equal(t, "admin", catalog.Roles[0].ID)

	i18nCatalog := i18nCatalogWithCodes("perm.audit.logs.read", "role.admin")
	require.NoError(t, ValidatePermissionCatalog(catalog, i18nCatalog))
}

func TestDecodePermissionModulesRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodePermissionModules([]byte(`{"modules":[],"extra":true}`))
	require.Error(t, err)
}

func TestValidatePermissionCatalogRejectsUnknownPermissionInRole(t *testing.T) {
	t.Parallel()

	catalog := PermissionCatalog{
		Modules: []PermissionModule{{
			ID:          "audit",
			Description: "Audit",
			Entries: []PermissionEntry{
				{Resource: "logs", Action: "read", Description: "View logs", I18nCode: "perm.audit.logs.read"},
			},
		}},
		Roles: []PermissionRole{
			{ID: "admin", Description: "Admin", I18nCode: "role.admin", Permissions: []string{"audit.logs.write"}},
		},
	}

	err := ValidatePermissionCatalog(catalog, i18nCatalogWithCodes("perm.audit.logs.read", "role.admin"))
	require.ErrorContains(t, err, "unknown permission")
}

func TestValidatePermissionCatalogRejectsUnknownI18nCode(t *testing.T) {
	t.Parallel()

	catalog := PermissionCatalog{
		Modules: []PermissionModule{{
			ID:          "audit",
			Description: "Audit",
			Entries: []PermissionEntry{
				{Resource: "logs", Action: "read", Description: "View logs", I18nCode: "perm.audit.logs.read"},
			},
		}},
		Roles: []PermissionRole{{ID: "admin", Description: "Admin", I18nCode: "role.admin"}},
	}

	err := ValidatePermissionCatalog(catalog, i18nCatalogWithCodes("role.admin"))
	require.ErrorContains(t, err, "unknown i18n code")
}

func TestValidatePermissionCatalogRejectsDuplicateModule(t *testing.T) {
	t.Parallel()

	catalog := PermissionCatalog{
		Modules: []PermissionModule{
			{ID: "audit", Description: "Audit"},
			{ID: "audit", Description: "Audit again"},
		},
		Roles: []PermissionRole{{ID: "admin", Description: "Admin", I18nCode: "role.admin"}},
	}

	err := ValidatePermissionCatalog(catalog, i18nCatalogWithCodes("role.admin"))
	require.ErrorContains(t, err, "duplicate permission module")
}
