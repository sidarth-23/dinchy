package main

import (
	"flag"
	"io"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func runValidatePermission(args []string) error {
	fs := flag.NewFlagSet("permission", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	permissionsInput := fs.String("permissions-input", "internal/access/permission/permissions.json", "permissions manifest input path")
	rolesInput := fs.String("roles-input", "internal/access/permission/roles.json", "roles manifest input path")
	i18nInput := fs.String("i18n-input", "internal/i18n/catalog", "i18n manifest input path (directory of fragments or single file)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, err := manifest.LoadPermissionCatalog(*permissionsInput, *rolesInput)
	if err != nil {
		return err
	}
	i18nCatalog, err := manifest.LoadI18nCatalog(*i18nInput)
	if err != nil {
		return err
	}
	return manifest.ValidatePermissionCatalog(catalog, i18nCatalog)
}
