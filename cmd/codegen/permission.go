package main

import (
	"flag"
	"io"
	"sort"
	"strings"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func runPermission(args []string) error {
	fs := flag.NewFlagSet("permission", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	permissionsInput := fs.String("permissions-input", "permissions.json", "permissions manifest input path")
	rolesInput := fs.String("roles-input", "roles.json", "roles manifest input path")
	i18nInput := fs.String("i18n-input", "../../i18n/catalog", "i18n manifest input path (directory of fragments or single file)")
	permissionsOutput := fs.String("permissions-output", "permission_generated.go", "generated permissions Go output path")
	rolesOutput := fs.String("roles-output", "role_generated.go", "generated roles Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return generatePermission(*permissionsInput, *rolesInput, *i18nInput, *permissionsOutput, *rolesOutput)
}

func generatePermission(permissionsPath, rolesPath, i18nPath, permissionsOutput, rolesOutput string) error {
	catalog, err := manifest.LoadPermissionCatalog(permissionsPath, rolesPath)
	if err != nil {
		return err
	}
	i18nCatalog, err := manifest.LoadI18nCatalog(i18nPath)
	if err != nil {
		return err
	}
	if err := manifest.ValidatePermissionCatalog(catalog, i18nCatalog); err != nil {
		return err
	}
	permissionsSource, err := renderPermissionsFile(catalog)
	if err != nil {
		return err
	}
	if err := writeGeneratedFile(permissionsOutput, permissionsSource); err != nil {
		return err
	}
	rolesSource, err := renderRolesFile(catalog)
	if err != nil {
		return err
	}
	return writeGeneratedFile(rolesOutput, rolesSource)
}

type (
	permissionsFileView struct{ Permissions []permissionView }
	rolesFileView       struct {
		Roles     []roleView
		RoleNames string
	}
)

type (
	permissionView struct{ ConstantName, Key, Module, Resource, Action, Description, I18nCode string }
	roleView       struct{ ConstantName, ID, Description, I18nCode, Permissions string }
)

func permissionViews(catalog manifest.PermissionCatalog) []permissionView {
	views := make([]permissionView, 0)
	for _, module := range catalog.Modules {
		for _, entry := range module.Entries {
			key := manifest.PermissionKey(module.ID, entry.Resource, entry.Action)
			views = append(views, permissionView{manifest.GoName(key), key, module.ID, entry.Resource, entry.Action, entry.Description, entry.I18nCode})
		}
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Key < views[j].Key })
	return views
}

func renderPermissionsFile(catalog manifest.PermissionCatalog) ([]byte, error) {
	return renderTemplate("permission.go.tmpl", permissionsFileView{Permissions: permissionViews(catalog)})
}

func renderRolesFile(catalog manifest.PermissionCatalog) ([]byte, error) {
	view := rolesFileView{}
	roles := append([]manifest.PermissionRole(nil), catalog.Roles...)
	sort.Slice(roles, func(i, j int) bool { return roles[i].ID < roles[j].ID })
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		grants := make([]string, 0, len(role.Permissions))
		for _, grant := range role.Permissions {
			grants = append(grants, manifest.GoName(grant))
		}
		view.Roles = append(view.Roles, roleView{manifest.GoName(role.ID), role.ID, role.Description, role.I18nCode, strings.Join(grants, ", ")})
		names = append(names, "Role"+manifest.GoName(role.ID))
	}
	view.RoleNames = strings.Join(names, ", ")
	return renderTemplate("role.go.tmpl", view)
}
