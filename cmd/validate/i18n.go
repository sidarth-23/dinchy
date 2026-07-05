package main

import (
	"flag"
	"io"
	"os"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func runValidateI18n(args []string) error {
	fs := flag.NewFlagSet("i18n", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "catalog.json", "manifest input path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	catalog, err := manifest.DecodeI18nCatalog(raw)
	if err != nil {
		return err
	}
	return manifest.ValidateI18nCatalog(catalog)
}
