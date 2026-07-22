package main

import (
	"flag"
	"io"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

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

	view := eventFileView{Imports: eventImports(events)}
	for _, event := range events {
		view.Events = append(view.Events, eventViewFor(event))
	}
	return renderTemplate("event.go.tmpl", view)
}

type eventFileView struct {
	Imports []string
	Events  []eventView
}

type eventView struct {
	ConstantName     string
	Type             string
	ID               string
	PathLiteral      string
	Category         string
	Subcategory      string
	Action           string
	Outcome          string
	Description      string
	MetadataKeyNames string
	ChangeKeyNames   string
	Metadata         eventRecordView
	Changes          eventRecordView
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
	return eventView{
		ConstantName:     event.ConstantName,
		Type:             event.Type,
		ID:               event.ID,
		PathLiteral:      quoteJoin(event.Path),
		Category:         eventCategory(event.Path),
		Subcategory:      eventSubcategory(event.Path),
		Action:           event.Action,
		Outcome:          event.Outcome,
		Description:      event.Description,
		MetadataKeyNames: renderStringNames(event.MetadataKeys),
		ChangeKeyNames:   renderStringNames(event.ChangeKeys),
		Metadata:         eventRecordViewFor(event.ConstantName+"Metadata", event.ConstantName+"MetadataKey", "New"+event.ConstantName+"Metadata", event.MetadataKeys),
		Changes:          eventRecordViewFor(event.ConstantName+"Changes", event.ConstantName+"ChangesKey", "New"+event.ConstantName+"Changes", event.ChangeKeys),
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
			GoType:       eventKeyGoType(key.Type),
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
	Category     string
	Subcategory  string
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
				Category:     eventCategory(currentPath),
				Subcategory:  eventSubcategory(currentPath),
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

func renderStringNames(keys []eventField) string {
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = key.Name
	}
	return quoteJoin(names)
}

func eventCategory(path []string) string {
	if len(path) > 1 {
		return path[1]
	}
	return ""
}

func eventSubcategory(path []string) string {
	if len(path) > 2 {
		return path[2]
	}
	return ""
}

func quoteJoin(values []string) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Quote(value)
	}
	return strings.Join(parts, ", ")
}

// eventImports lists the imports the generated events file needs. The Envelope
// type always carries a time.Time, so time is always required.
func eventImports([]flattenedEvent) []string {
	return []string{"time"}
}

func eventKeyGoType(value string) string {
	switch value {
	case "string", "bool", "int", "int64", "float64", "time.Time":
		return value
	default:
		return "string"
	}
}
