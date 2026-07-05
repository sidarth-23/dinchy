package main

import (
	"flag"
	"io"
	"os"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func runValidateError(args []string) error {
	fs := flag.NewFlagSet("error", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "catalog.json", "manifest input path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	catalog, err := manifest.DecodeErrorCatalog(raw)
	if err != nil {
		return err
	}
	return manifest.ValidateErrorCatalog(catalog)
}
