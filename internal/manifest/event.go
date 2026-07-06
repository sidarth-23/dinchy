package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type EventCatalog struct {
	Modules []EventModule `json:"modules"`
}

type EventModule struct {
	ID          string            `json:"id"`
	Description string            `json:"description,omitempty"`
	Modules     []EventModule     `json:"modules,omitempty"`
	Events      []EventDefinition `json:"events,omitempty"`
}

type EventDefinition struct {
	ID           string     `json:"id"`
	Description  string     `json:"description,omitempty"`
	Action       string     `json:"action"`
	Outcome      string     `json:"outcome"`
	MetadataKeys []Field `json:"metadata_keys,omitempty"`
	ChangeKeys   []Field `json:"change_keys,omitempty"`
}

type Field struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

func DecodeEventCatalog(raw []byte) (EventCatalog, error) {
	var catalog EventCatalog
	if err := decodeStrict(raw, &catalog); err != nil {
		return EventCatalog{}, err
	}
	return catalog, nil
}

func ValidateEventCatalog(catalog EventCatalog) error {
	if len(catalog.Modules) == 0 {
		return fmt.Errorf("event catalog must define at least one module")
	}

	seenTypes := map[string]struct{}{}
	seenNames := map[string]struct{}{}
	return validateEventModules(catalog.Modules, nil, seenTypes, seenNames)
}

func validateEventModules(modules []EventModule, modulePath []string, seenTypes, seenNames map[string]struct{}) error {
	seenModuleIDs := map[string]struct{}{}
	for _, module := range modules {
		if module.ID == "" {
			return fmt.Errorf("module id cannot be empty")
		}
		if _, ok := seenModuleIDs[module.ID]; ok {
			return fmt.Errorf("duplicate module id %q within module path %q", module.ID, DisplayPath(modulePath))
		}
		seenModuleIDs[module.ID] = struct{}{}

		currentPath := append(append([]string{}, modulePath...), module.ID)
		seenEventIDs := map[string]struct{}{}
		for _, event := range module.Events {
			if event.ID == "" {
				return fmt.Errorf("event id cannot be empty within module path %q", DisplayPath(currentPath))
			}
			if _, ok := seenEventIDs[event.ID]; ok {
				return fmt.Errorf("duplicate event id %q within module path %q", event.ID, DisplayPath(currentPath))
			}
			if event.Action == "" {
				return fmt.Errorf("event %q has empty action", DisplayPath(append(currentPath, event.ID)))
			}
			if event.Outcome == "" {
				return fmt.Errorf("event %q has empty outcome", DisplayPath(append(currentPath, event.ID)))
			}
			fullType := EventTypeFor(currentPath, event.ID)
			if _, ok := seenTypes[fullType]; ok {
				return fmt.Errorf("duplicate event type %q", fullType)
			}
			constName := EventConstantName(currentPath, event.ID)
			if _, ok := seenNames[constName]; ok {
				return fmt.Errorf("duplicate generated constant name %q", constName)
			}
			if err := validateTypedFields(fullType, "metadata_keys", event.MetadataKeys); err != nil {
				return err
			}
			if err := validateTypedFields(fullType, "change_keys", event.ChangeKeys); err != nil {
				return err
			}
			seenEventIDs[event.ID] = struct{}{}
			seenTypes[fullType] = struct{}{}
			seenNames[constName] = struct{}{}
		}

		if err := validateEventModules(module.Modules, currentPath, seenTypes, seenNames); err != nil {
			return err
		}
	}
	return nil
}

func validateTypedFields(eventType, field string, fields []Field) error {
	seenKeys := map[string]struct{}{}
	for _, fieldValue := range fields {
		if fieldValue.Name == "" {
			return fmt.Errorf("event %q has empty %s key", eventType, field)
		}
		if fieldValue.Type == "" {
			return fmt.Errorf("event %q has empty %s type for %q", eventType, field, fieldValue.Name)
		}
		if _, ok := seenKeys[fieldValue.Name]; ok {
			return fmt.Errorf("event %q has duplicate %s key %q", eventType, field, fieldValue.Name)
		}
		if !supportedEventTypedFieldType(fieldValue.Type) {
			return fmt.Errorf("event %q has unsupported %s type %q for %q", eventType, field, fieldValue.Type, fieldValue.Name)
		}
		seenKeys[fieldValue.Name] = struct{}{}
	}
	return nil
}

func EventTypeFor(modulePath []string, eventID string) string {
	parts := append(append([]string{}, modulePath...), eventID)
	return DisplayPath(parts)
}

func EventConstantName(modulePath []string, eventID string) string {
	parts := append(append([]string{}, modulePath...), eventID)
	var name strings.Builder
	for _, part := range parts {
		name.WriteString(GoName(part))
	}
	return name.String()
}

func supportedEventTypedFieldType(value string) bool {
	switch value {
	case "string", "bool", "int", "int64", "float64", "time.Time":
		return true
	default:
		return false
	}
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := ensureEOF(decoder); err != nil {
		return err
	}
	return nil
}

func ensureEOF(decoder *json.Decoder) error {
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing data after JSON document")
		}
		return err
	}
	return nil
}
