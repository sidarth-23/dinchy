package main

import (
	"flag"
	"io"
	"os"
	"sort"
	"strings"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func runPermission(args []string) error {
	fs := flag.NewFlagSet("permission", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "catalog.json", "permission manifest input path")
	i18nInput := fs.String("i18n-input", "internal/i18n/catalog", "i18n manifest input path (directory of fragments or single file)")
	output := fs.String("output", "permission_type.go", "generated Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return generatePermission(*input, *i18nInput, *output)
}

func generatePermission(inputPath, i18nPath, outputPath string) error {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	catalog, err := manifest.DecodePermissionCatalog(raw)
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
	source, err := renderPermissionManifest(catalog)
	if err != nil {
		return err
	}
	return writeGeneratedFile(outputPath, source)
}

type permissionFileView struct {
	Permissions []permissionView
	Roles       []roleView
	RoleNames   string
}
type permissionView struct{ ConstantName, Key, Module, Resource, Action, Description, I18nCode string }
type roleView struct{ ConstantName, ID, Description, I18nCode, Permissions string }

func renderPermissionManifest(catalog manifest.PermissionCatalog) ([]byte, error) {
	view := permissionFileView{}
	for _, module := range catalog.Modules {
		for _, entry := range module.Entries {
			key := manifest.PermissionKey(module.ID, entry.Resource, entry.Action)
			view.Permissions = append(view.Permissions, permissionView{manifest.GoName(key), key, module.ID, entry.Resource, entry.Action, entry.Description, entry.I18nCode})
		}
	}
	sort.Slice(view.Permissions, func(i, j int) bool { return view.Permissions[i].Key < view.Permissions[j].Key })
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
	return renderTemplate("permission.go.tmpl", view)
}
