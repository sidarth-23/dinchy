package main

import (
	"flag"
	"io"
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
	input := fs.String("input", "catalog", "manifest input path (directory of fragments or single file)")
	output := fs.String("output", "generated.go", "generated Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return generateEvent(*input, *output)
}

func generateEvent(inputPath, outputPath string) error {
	mf, err := manifest.LoadEventCatalog(inputPath)
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

func eventViewFor(event flattenedEvent) eventView {
	var category, subcategory string
	if len(event.Path) > 1 {
		category = event.Path[1]
	}
	if len(event.Path) > 2 {
		subcategory = event.Path[2]
	}
	hasChanges := len(event.ChangeKeys) > 0
	changesType := "NoChanges"
	if hasChanges {
		changesType = event.ConstantName + "Changes"
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
		Metadata:     eventRecordViewFor(event.ConstantName+"Metadata", "New"+event.ConstantName+"Metadata", event.MetadataKeys),
		Changes:      eventRecordViewFor(event.ConstantName+"Changes", "New"+event.ConstantName+"Changes", event.ChangeKeys),
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
