package main

import (
	"flag"
	"io"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func runValidateI18n(args []string) error {
	fs := flag.NewFlagSet("i18n", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "catalog", "manifest input path (directory of fragments or single file)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	catalog, err := manifest.LoadI18nCatalog(*input)
	if err != nil {
		return err
	}
	return manifest.ValidateI18nCatalog(catalog)
}
