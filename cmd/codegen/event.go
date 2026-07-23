package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"sort"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

type (
	eventManifest   = manifest.EventCatalog
	eventModule     = manifest.EventModule
	eventDefinition = manifest.EventDefinition
	eventField      = manifest.Field
)

func runEvent(args []string) error {
	fs := flag.NewFlagSet("event", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	features := fs.String("features", "internal/features", "feature packages directory holding per-feature events.json fragments")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return generateEvent(*features)
}

// generateEvent discovers every feature's events.json fragment, validates them
// as one catalog so event types and generated names stay globally unique, then
// writes a per-feature events_generated.go beside each fragment.
func generateEvent(featuresDir string) error {
	fragments, err := loadEventFragments(featuresDir)
	if err != nil {
		return err
	}
	merged, err := manifest.MergeEventCatalogs(fragmentCatalogs(fragments)...)
	if err != nil {
		return err
	}
	if err := manifest.ValidateEventCatalog(merged); err != nil {
		return err
	}
	for _, fragment := range fragments {
		src, err := renderEventManifest(fragment.catalog, fragment.pkg)
		if err != nil {
			return err
		}
		if err := writeGeneratedFile(filepath.Join(fragment.dir, "events_generated.go"), src); err != nil {
			return err
		}
	}
	return nil
}

type eventFragment struct {
	pkg     string
	dir     string
	catalog eventManifest
}

func loadEventFragments(featuresDir string) ([]eventFragment, error) {
	matches, err := filepath.Glob(filepath.Join(featuresDir, "*", "events.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no event catalog fragments found under %q", featuresDir)
	}
	fragments := make([]eventFragment, 0, len(matches))
	for _, path := range matches {
		catalog, err := manifest.LoadEventCatalog(path)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(path)
		pkg := filepath.Base(dir)
		if len(catalog.Modules) != 1 || catalog.Modules[0].ID != pkg {
			return nil, fmt.Errorf("event fragment %q must contain exactly one top-level module named %q", path, pkg)
		}
		fragments = append(fragments, eventFragment{pkg: pkg, dir: dir, catalog: catalog})
	}
	return fragments, nil
}

func fragmentCatalogs(fragments []eventFragment) []eventManifest {
	catalogs := make([]eventManifest, 0, len(fragments))
	for _, fragment := range fragments {
		catalogs = append(catalogs, fragment.catalog)
	}
	return catalogs
}

func renderEventManifest(mf eventManifest, pkg string) ([]byte, error) {
	events := flattenEventDefinitions(mf.Modules, nil)
	sort.Slice(events, func(i, j int) bool { return events[i].Type < events[j].Type })

	view := eventFileView{Package: pkg}
	for i := range events {
		ev := eventViewFor(events[i])
		view.Events = append(view.Events, ev)
		if eventViewNeedsTime(ev) {
			view.NeedsTime = true
		}
	}
	return renderTemplate("event.go.tmpl", view)
}

type eventFileView struct {
	Package   string
	Events    []eventView
	NeedsTime bool
}

type eventView struct {
	ConstantName string
	Type         string
	ID           string
	Path         []string
	Category     string
	Subcategory  string
	Action       string
	Outcome      string
	Description  string
	Metadata     eventRecordView
	Changes      eventRecordView
	HasChanges   bool
	ChangesType  string
}

type eventRecordView struct {
	TypeName        string
	ConstructorName string
	Fields          []eventFieldView
}

type eventFieldView struct {
	RawName       string
	GoName        string
	GoType        string
	ParamName     string
	ParamPrevious string
	ParamCurrent  string
}

// eventViewFor builds the template view for one event. The constant and type
// names drop the leading module segment (the feature that owns the generated
// package) so identifiers do not stutter with the package name; the catalog
// type string and the definition path keep the full module path.
func eventViewFor(event flattenedEvent) eventView {
	var category, subcategory string
	if len(event.Path) > 1 {
		category = event.Path[1]
	}
	if len(event.Path) > 2 {
		subcategory = event.Path[2]
	}
	localPath := event.Path
	if len(localPath) > 0 {
		localPath = localPath[1:]
	}
	constantName := manifest.EventConstantName(localPath, event.ID)
	hasChanges := len(event.ChangeKeys) > 0
	changesType := "events.NoChanges"
	if hasChanges {
		changesType = constantName + "Changes"
	}
	return eventView{
		ConstantName: constantName,
		Type:         event.Type,
		ID:           event.ID,
		Path:         event.Path,
		Category:     category,
		Subcategory:  subcategory,
		Action:       event.Action,
		Outcome:      event.Outcome,
		Description:  event.Description,
		Metadata:     eventRecordViewFor(constantName+"Metadata", "New"+constantName+"Metadata", event.MetadataKeys),
		Changes:      eventRecordViewFor(constantName+"Changes", "New"+constantName+"Changes", event.ChangeKeys),
		HasChanges:   hasChanges,
		ChangesType:  changesType,
	}
}

func eventRecordViewFor(typeName, constructorName string, keys []eventField) eventRecordView {
	record := eventRecordView{
		TypeName:        typeName,
		ConstructorName: constructorName,
	}
	for _, key := range keys {
		goName := manifest.GoName(key.Name)
		paramName := manifest.LowerCamel(goName)
		record.Fields = append(record.Fields, eventFieldView{
			RawName:       key.Name,
			GoName:        goName,
			GoType:        key.Type,
			ParamName:     paramName,
			ParamPrevious: paramName + "Previous",
			ParamCurrent:  paramName + "Current",
		})
	}
	return record
}

func eventViewNeedsTime(event eventView) bool {
	for _, field := range event.Metadata.Fields {
		if field.GoType == "time.Time" {
			return true
		}
	}
	if event.HasChanges {
		for _, field := range event.Changes.Fields {
			if field.GoType == "time.Time" {
				return true
			}
		}
	}
	return false
}

type flattenedEvent struct {
	ID           string
	Type         string
	Path         []string
	Action       string
	Outcome      string
	Description  string
	MetadataKeys []eventField
	ChangeKeys   []eventField
	ConstantName string
}

func flattenEventDefinitions(modules []eventModule, modulePath []string) []flattenedEvent {
	out := make([]flattenedEvent, 0)
	for _, module := range modules {
		currentPath := append(append([]string{}, modulePath...), module.ID)
		for _, event := range module.Events {
			out = append(out, flattenedEvent{
				ID:           event.ID,
				Type:         manifest.EventTypeFor(currentPath, event.ID),
				Path:         append([]string{}, currentPath...),
				Action:       event.Action,
				Outcome:      event.Outcome,
				Description:  event.Description,
				MetadataKeys: slices.Clone(event.MetadataKeys),
				ChangeKeys:   slices.Clone(event.ChangeKeys),
				ConstantName: manifest.EventConstantName(currentPath, event.ID),
			})
		}
		out = append(out, flattenEventDefinitions(module.Modules, currentPath)...)
	}
	return out
}
