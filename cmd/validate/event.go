package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func runValidateEvent(args []string) error {
	fs := flag.NewFlagSet("event", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	features := fs.String("features", "internal/features", "feature packages directory holding per-feature events.json fragments")
	if err := fs.Parse(args); err != nil {
		return err
	}
	matches, err := filepath.Glob(filepath.Join(*features, "*", "events.json"))
	if err != nil {
		return err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return fmt.Errorf("no event catalog fragments found under %q", *features)
	}
	catalogs := make([]manifest.EventCatalog, 0, len(matches))
	for _, path := range matches {
		catalog, err := manifest.LoadEventCatalog(path)
		if err != nil {
			return err
		}
		catalogs = append(catalogs, catalog)
	}
	merged, err := manifest.MergeEventCatalogs(catalogs...)
	if err != nil {
		return err
	}
	return manifest.ValidateEventCatalog(merged)
}
