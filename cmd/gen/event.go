package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"sort"
	"strings"
)

type eventManifest struct {
	Subscribers []eventSubscriber `json:"subscribers"`
	Modules     []eventModule     `json:"modules"`
}

type eventSubscriber struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type eventModule struct {
	ID          string            `json:"id"`
	Description string            `json:"description,omitempty"`
	Modules     []eventModule     `json:"modules,omitempty"`
	Events      []eventDefinition `json:"events,omitempty"`
}

type eventDefinition struct {
	ID           string     `json:"id"`
	Description  string     `json:"description,omitempty"`
	Subscriber   string     `json:"subscriber"`
	Category     string     `json:"category"`
	Subcategory  string     `json:"subcategory"`
	Action       string     `json:"action"`
	Outcome      string     `json:"outcome"`
	MetadataKeys []eventKey `json:"metadata_keys,omitempty"`
	ChangeKeys   []eventKey `json:"change_keys,omitempty"`
}

type eventKey struct {
	Key    string `json:"key"`
	GoType string `json:"go_type,omitempty"`
}

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

	var mf eventManifest
	if err := json.Unmarshal(raw, &mf); err != nil {
		return err
	}
	if err := validateEventManifest(mf); err != nil {
		return err
	}

	src, err := renderEventManifest(mf)
	if err != nil {
		return err
	}
	return writeGeneratedFile(outputPath, src)
}

func validateEventManifest(mf eventManifest) error {
	seenSubscribers := map[string]struct{}{}
	for _, subscriber := range mf.Subscribers {
		if subscriber.Name == "" {
			return fmt.Errorf("subscriber name cannot be empty")
		}
		if _, ok := seenSubscribers[subscriber.Name]; ok {
			return fmt.Errorf("duplicate subscriber name %q", subscriber.Name)
		}
		seenSubscribers[subscriber.Name] = struct{}{}
	}

	if len(mf.Modules) == 0 {
		return fmt.Errorf("event catalog must define at least one module")
	}

	seenTypes := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	return validateEventModules(mf.Modules, nil, seenSubscribers, seenTypes, seenNames)
}

func validateEventModules(modules []eventModule, modulePath []string, seenSubscribers, seenTypes, seenNames map[string]struct{}) error {
	seenModuleIDs := map[string]struct{}{}
	for _, module := range modules {
		if module.ID == "" {
			return fmt.Errorf("module id cannot be empty")
		}
		if _, ok := seenModuleIDs[module.ID]; ok {
			return fmt.Errorf("duplicate module id %q within module path %q", module.ID, displayPath(modulePath))
		}
		seenModuleIDs[module.ID] = struct{}{}

		currentPath := append(append([]string{}, modulePath...), module.ID)
		seenEventIDs := map[string]struct{}{}
		for _, event := range module.Events {
			if event.ID == "" {
				return fmt.Errorf("event id cannot be empty within module path %q", displayPath(currentPath))
			}
			if _, ok := seenEventIDs[event.ID]; ok {
				return fmt.Errorf("duplicate event id %q within module path %q", event.ID, displayPath(currentPath))
			}
			if event.Subscriber == "" {
				return fmt.Errorf("event %q has empty subscriber", displayPath(append(currentPath, event.ID)))
			}
			if event.Category == "" {
				return fmt.Errorf("event %q has empty category", displayPath(append(currentPath, event.ID)))
			}
			if event.Subcategory == "" {
				return fmt.Errorf("event %q has empty subcategory", displayPath(append(currentPath, event.ID)))
			}
			if event.Action == "" {
				return fmt.Errorf("event %q has empty action", displayPath(append(currentPath, event.ID)))
			}
			if event.Outcome == "" {
				return fmt.Errorf("event %q has empty outcome", displayPath(append(currentPath, event.ID)))
			}
			if _, ok := seenSubscribers[event.Subscriber]; !ok {
				return fmt.Errorf("event %q references unknown subscriber %q", displayPath(append(currentPath, event.ID)), event.Subscriber)
			}
			fullType := eventTypeFor(currentPath, event.ID)
			if _, ok := seenTypes[fullType]; ok {
				return fmt.Errorf("duplicate event type %q", fullType)
			}
			constName := eventConstantName(currentPath, event.ID)
			if _, ok := seenNames[constName]; ok {
				return fmt.Errorf("duplicate generated constant name %q", constName)
			}
			if err := validateTypedKeys(fullType, "metadata_keys", event.MetadataKeys); err != nil {
				return err
			}
			if err := validateTypedKeys(fullType, "change_keys", event.ChangeKeys); err != nil {
				return err
			}
			seenEventIDs[event.ID] = struct{}{}
			seenTypes[fullType] = struct{}{}
			seenNames[constName] = struct{}{}
		}

		if err := validateEventModules(module.Modules, currentPath, seenSubscribers, seenTypes, seenNames); err != nil {
			return err
		}
	}
	return nil
}

func validateTypedKeys(eventType, field string, keys []eventKey) error {
	seenKeys := map[string]struct{}{}
	for _, key := range keys {
		if key.Key == "" {
			return fmt.Errorf("event %q has empty %s key", eventType, field)
		}
		if _, ok := seenKeys[key.Key]; ok {
			return fmt.Errorf("event %q has duplicate %s key %q", eventType, field, key.Key)
		}
		seenKeys[key.Key] = struct{}{}
	}
	return nil
}

func renderEventManifest(mf eventManifest) ([]byte, error) {
	events := flattenEventDefinitions(mf.Modules, nil)
	sort.Slice(events, func(i, j int) bool { return events[i].Type < events[j].Type })

	var b strings.Builder
	b.WriteString("// Code generated by cmd/gen event; DO NOT EDIT.\n\n")
	b.WriteString("package events\n\n")
	b.WriteString("type Type string\n\n")
	b.WriteString("type Subscriber string\n\n")
	b.WriteString("type TypedKey struct {\n")
	b.WriteString("\tKey string\n")
	b.WriteString("\tGoType string\n")
	b.WriteString("}\n\n")
	b.WriteString("const (\n")
	for _, subscriber := range mf.Subscribers {
		fmt.Fprintf(&b, "\tSubscriber%s Subscriber = %q\n", subscriberName(subscriber.Name), subscriber.Name)
	}
	b.WriteString(")\n\n")
	b.WriteString("type Definition struct {\n")
	b.WriteString("\tID string\n")
	b.WriteString("\tType Type\n")
	b.WriteString("\tModule string\n")
	b.WriteString("\tSubscriber Subscriber\n")
	b.WriteString("\tCategory string\n")
	b.WriteString("\tSubcategory string\n")
	b.WriteString("\tAction string\n")
	b.WriteString("\tOutcome string\n")
	b.WriteString("\tDescription string\n")
	b.WriteString("\tMetadataKeys []TypedKey\n")
	b.WriteString("\tChangeKeys []TypedKey\n")
	b.WriteString("}\n\n")
	b.WriteString("const (\n")
	for _, event := range events {
		fmt.Fprintf(&b, "\t%s Type = %q\n", event.ConstantName, event.Type)
	}
	b.WriteString(")\n\n")
	b.WriteString("var Definitions = map[Type]Definition{\n")
	for _, event := range events {
		subscriberConst := "Subscriber" + subscriberName(event.Subscriber)
		fmt.Fprintf(&b, "\t%s: {\n", event.ConstantName)
		fmt.Fprintf(&b, "\t\tID: %q,\n", event.ID)
		fmt.Fprintf(&b, "\t\tType: %s,\n", event.ConstantName)
		fmt.Fprintf(&b, "\t\tModule: %q,\n", event.Module)
		fmt.Fprintf(&b, "\t\tSubscriber: %s,\n", subscriberConst)
		fmt.Fprintf(&b, "\t\tCategory: %q,\n", event.Category)
		fmt.Fprintf(&b, "\t\tSubcategory: %q,\n", event.Subcategory)
		fmt.Fprintf(&b, "\t\tAction: %q,\n", event.Action)
		fmt.Fprintf(&b, "\t\tOutcome: %q,\n", event.Outcome)
		if event.Description != "" {
			fmt.Fprintf(&b, "\t\tDescription: %q,\n", event.Description)
		}
		if len(event.MetadataKeys) > 0 {
			fmt.Fprintf(&b, "\t\tMetadataKeys: []TypedKey{%s},\n", renderTypedKeys(event.MetadataKeys))
		}
		if len(event.ChangeKeys) > 0 {
			fmt.Fprintf(&b, "\t\tChangeKeys: []TypedKey{%s},\n", renderTypedKeys(event.ChangeKeys))
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("var SubscriberDefinitions = map[Subscriber][]Type{\n")
	for _, subscriber := range mf.Subscribers {
		subscriberConst := "Subscriber" + subscriberName(subscriber.Name)
		b.WriteString("\t")
		b.WriteString(subscriberConst)
		b.WriteString(": {\n")
		for _, event := range events {
			if event.Subscriber != subscriber.Name {
				continue
			}
			fmt.Fprintf(&b, "\t\t%s,\n", event.ConstantName)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("func DefinitionFor(eventType Type) (Definition, bool) {\n")
	b.WriteString("\tdefinition, ok := Definitions[eventType]\n")
	b.WriteString("\treturn definition, ok\n")
	b.WriteString("}\n\n")
	b.WriteString("func EventsForSubscriber(subscriber Subscriber) []Definition {\n")
	b.WriteString("\ttypes := SubscriberDefinitions[subscriber]\n")
	b.WriteString("\tout := make([]Definition, 0, len(types))\n")
	b.WriteString("\tfor _, eventType := range types {\n")
	b.WriteString("\t\tif definition, ok := Definitions[eventType]; ok {\n")
	b.WriteString("\t\t\tout = append(out, definition)\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn out\n")
	b.WriteString("}\n")

	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return nil, err
	}
	return formatted, nil
}

type flattenedEvent struct {
	ID           string
	Type         string
	Module       string
	Subscriber   string
	Category     string
	Subcategory  string
	Action       string
	Outcome      string
	Description  string
	MetadataKeys []eventKey
	ChangeKeys   []eventKey
	ConstantName string
}

func flattenEventDefinitions(modules []eventModule, modulePath []string) []flattenedEvent {
	out := make([]flattenedEvent, 0)
	for _, module := range modules {
		currentPath := append(append([]string{}, modulePath...), module.ID)
		for _, event := range module.Events {
			out = append(out, flattenedEvent{
				ID:           event.ID,
				Type:         eventTypeFor(currentPath, event.ID),
				Module:       displayPath(currentPath),
				Subscriber:   event.Subscriber,
				Category:     event.Category,
				Subcategory:  event.Subcategory,
				Action:       event.Action,
				Outcome:      event.Outcome,
				Description:  event.Description,
				MetadataKeys: normalizeTypedKeys(event.MetadataKeys),
				ChangeKeys:   normalizeTypedKeys(event.ChangeKeys),
				ConstantName: eventConstantName(currentPath, event.ID),
			})
		}
		out = append(out, flattenEventDefinitions(module.Modules, currentPath)...)
	}
	return out
}

func normalizeTypedKeys(keys []eventKey) []eventKey {
	if len(keys) == 0 {
		return nil
	}
	out := make([]eventKey, len(keys))
	copy(out, keys)
	for i := range out {
		if out[i].GoType == "" {
			out[i].GoType = "string"
		}
	}
	return out
}

func renderTypedKeys(keys []eventKey) string {
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		goType := key.GoType
		if goType == "" {
			goType = "string"
		}
		parts = append(parts, fmt.Sprintf("{Key: %q, GoType: %q}", key.Key, goType))
	}
	return strings.Join(parts, ", ")
}

func displayPath(parts []string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return "<root>"
	}
	return strings.Join(cleaned, ".")
}

func eventTypeFor(modulePath []string, eventID string) string {
	parts := append(append([]string{}, modulePath...), eventID)
	return displayPath(parts)
}

func eventConstantName(modulePath []string, eventID string) string {
	parts := append(append([]string{}, modulePath...), eventID)
	var name strings.Builder
	for _, part := range parts {
		name.WriteString(goName(part))
	}
	return name.String()
}

func goName(value string) string {
	segments := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	})
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		segments[i] = strings.ToUpper(segment[:1]) + strings.ToLower(segment[1:])
	}
	return strings.Join(segments, "")
}

func subscriberName(value string) string {
	return goName(value)
}
