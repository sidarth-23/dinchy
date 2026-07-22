package main

import (
	"flag"
	"io"
	"os"
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
	input := fs.String("input", "catalog.json", "manifest input path")
	output := fs.String("output", "generated.go", "generated Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return generateEvent(*input, *output)
}

func generateEvent(inputPath, outputPath string) error {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	mf, err := manifest.DecodeEventCatalog(raw)
	if err != nil {
		return err
	}
	if err := manifest.ValidateEventCatalog(mf); err != nil {
		return err
	}

	src, err := renderEventManifest(mf)
	if err != nil {
		return err
	}
	return writeGeneratedFile(outputPath, src)
}

func renderEventManifest(mf eventManifest) ([]byte, error) {
	events := flattenEventDefinitions(mf.Modules, nil)
	sort.Slice(events, func(i, j int) bool { return events[i].Type < events[j].Type })

	view := eventFileView{}
	for _, event := range events {
		view.Events = append(view.Events, eventViewFor(event))
	}
	return renderTemplate("event.go.tmpl", view)
}

type eventFileView struct {
	Events []eventView
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
}

type eventRecordView struct {
	KeyTypeName     string
	TypeName        string
	ConstructorName string
	Fields          []eventFieldView
}

type eventFieldView struct {
	RawName      string
	GoName       string
	GoType       string
	ParamName    string
	KeyConstName string
}

func eventViewFor(event flattenedEvent) eventView {
	var category, subcategory string
	if len(event.Path) > 1 {
		category = event.Path[1]
	}
	if len(event.Path) > 2 {
		subcategory = event.Path[2]
	}
	return eventView{
		ConstantName: event.ConstantName,
		Type:         event.Type,
		ID:           event.ID,
		Path:         event.Path,
		Category:     category,
		Subcategory:  subcategory,
		Action:       event.Action,
		Outcome:      event.Outcome,
		Description:  event.Description,
		Metadata:     eventRecordViewFor(event.ConstantName+"Metadata", event.ConstantName+"MetadataKey", "New"+event.ConstantName+"Metadata", event.MetadataKeys),
		Changes:      eventRecordViewFor(event.ConstantName+"Changes", event.ConstantName+"ChangesKey", "New"+event.ConstantName+"Changes", event.ChangeKeys),
	}
}

func eventRecordViewFor(typeName, keyTypeName, constructorName string, keys []eventField) eventRecordView {
	record := eventRecordView{
		KeyTypeName:     keyTypeName,
		TypeName:        typeName,
		ConstructorName: constructorName,
	}
	for _, key := range keys {
		goName := manifest.GoName(key.Name)
		record.Fields = append(record.Fields, eventFieldView{
			RawName:      key.Name,
			GoName:       goName,
			GoType:       key.Type,
			ParamName:    manifest.LowerCamel(goName),
			KeyConstName: keyTypeName + goName,
		})
	}
	return record
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
