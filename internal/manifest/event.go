package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EventCatalog is the root of the event manifest.
type EventCatalog struct {
	Modules []EventModule `json:"modules"`
}

// EventModule is a namespace grouping event definitions and nested modules.
type EventModule struct {
	ID          string            `json:"id"`
	Description string            `json:"description,omitempty"`
	Modules     []EventModule     `json:"modules,omitempty"`
	Events      []EventDefinition `json:"events,omitempty"`
}

// EventDefinition describes a single event, its action and outcome, and its typed keys.
type EventDefinition struct {
	ID           string  `json:"id"`
	Description  string  `json:"description,omitempty"`
	Action       string  `json:"action"`
	Outcome      string  `json:"outcome"`
	MetadataKeys []Field `json:"metadata_keys,omitempty"`
	ChangeKeys   []Field `json:"change_keys,omitempty"`
}

// Field is a named, typed key on an event's metadata or changes.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// DecodeEventCatalog strictly decodes raw JSON into an EventCatalog.
func DecodeEventCatalog(raw []byte) (EventCatalog, error) {
	var catalog EventCatalog
	if err := decodeStrict(raw, &catalog); err != nil {
		return EventCatalog{}, err
	}
	return catalog, nil
}

// LoadEventCatalog loads a catalog from path, which may be either a directory of
// *.json fragments or a single fragment file. It decodes and merges but does not
// validate; callers validate the result.
func LoadEventCatalog(path string) (EventCatalog, error) {
	info, err := os.Stat(path)
	if err != nil {
		return EventCatalog{}, fmt.Errorf("stat event catalog %q: %w", path, err)
	}
	if info.IsDir() {
		return LoadEventCatalogDir(path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return EventCatalog{}, fmt.Errorf("read event catalog %q: %w", path, err)
	}
	return DecodeEventCatalog(raw)
}

// LoadEventCatalogDir reads every *.json fragment in dir (in sorted filename
// order), strictly decodes each, and deep-merges them into one catalog. It does
// not validate the result; callers validate the merged catalog. It errors if dir
// contains no fragments so an empty directory fails loudly at its source.
func LoadEventCatalogDir(dir string) (EventCatalog, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return EventCatalog{}, fmt.Errorf("scan event catalog directory %q: %w", dir, err)
	}
	if len(matches) == 0 {
		return EventCatalog{}, fmt.Errorf("no event fragments found in %q", dir)
	}
	sort.Strings(matches)

	catalogs := make([]EventCatalog, 0, len(matches))
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil {
			return EventCatalog{}, fmt.Errorf("read event fragment %q: %w", path, err)
		}
		catalog, err := DecodeEventCatalog(raw)
		if err != nil {
			return EventCatalog{}, fmt.Errorf("decode event fragment %q: %w", path, err)
		}
		catalogs = append(catalogs, catalog)
	}
	return MergeEventCatalogs(catalogs...)
}

// MergeEventCatalogs deep-merges catalogs by module ID at each level. Modules
// sharing an ID under the same parent are merged recursively; events are only
// concatenated, leaving duplicate detection to ValidateEventCatalog so its precise
// errors are preserved. Conflicting non-empty descriptions for the same module
// are an error.
func MergeEventCatalogs(catalogs ...EventCatalog) (EventCatalog, error) {
	var merged EventCatalog
	for _, catalog := range catalogs {
		modules, err := mergeEventModules(merged.Modules, catalog.Modules)
		if err != nil {
			return EventCatalog{}, err
		}
		merged.Modules = modules
	}
	return merged, nil
}

func mergeEventModules(dst, src []EventModule) ([]EventModule, error) {
	index := make(map[string]int, len(dst))
	for i, module := range dst {
		index[module.ID] = i
	}
	for _, module := range src {
		pos, ok := index[module.ID]
		if !ok {
			dst = append(dst, module)
			index[module.ID] = len(dst) - 1
			continue
		}
		combined, err := mergeEventModule(dst[pos], module)
		if err != nil {
			return nil, err
		}
		dst[pos] = combined
	}
	return dst, nil
}

func mergeEventModule(dst, src EventModule) (EventModule, error) {
	description, err := mergeEventDescription(dst.ID, dst.Description, src.Description)
	if err != nil {
		return EventModule{}, err
	}
	dst.Description = description
	dst.Events = append(append([]EventDefinition{}, dst.Events...), src.Events...)
	children, err := mergeEventModules(dst.Modules, src.Modules)
	if err != nil {
		return EventModule{}, err
	}
	dst.Modules = children
	return dst, nil
}

func mergeEventDescription(id, existing, incoming string) (string, error) {
	switch {
	case existing == "":
		return incoming, nil
	case incoming == "" || incoming == existing:
		return existing, nil
	default:
		return "", fmt.Errorf("conflicting descriptions for module %q", id)
	}
}

// ValidateEventCatalog reports whether the catalog is well-formed and free of
// duplicate module IDs, event types, generated constant names, and field keys.
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
		switch fieldValue.Type {
		case "string", "bool", "int", "int64", "float64", "time.Time":
		default:
			return fmt.Errorf("event %q has unsupported %s type %q for %q", eventType, field, fieldValue.Type, fieldValue.Name)
		}
		seenKeys[fieldValue.Name] = struct{}{}
	}
	return nil
}

// EventTypeFor returns the dotted event type string for a module path and ID.
func EventTypeFor(modulePath []string, eventID string) string {
	parts := append(append([]string{}, modulePath...), eventID)
	return DisplayPath(parts)
}

// EventConstantName returns the generated Go constant name for an event.
func EventConstantName(modulePath []string, eventID string) string {
	parts := append(append([]string{}, modulePath...), eventID)
	var name strings.Builder
	for _, part := range parts {
		name.WriteString(GoName(part))
	}
	return name.String()
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing data after JSON document")
		}
		return err
	}
	return nil
}
