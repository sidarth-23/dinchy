// Package manifest decodes and validates the error, i18n, and event catalogs
// that drive Dinchy's code generation, and derives the generated Go names,
// codes, and metadata keys from them.
package manifest

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ErrorCatalog is the root of the error manifest.
type ErrorCatalog struct {
	Modules []ErrorModule `json:"modules"`
}

// ErrorModule is a top-level namespace grouping error nodes.
type ErrorModule struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Modules     []ErrorNode `json:"modules,omitempty"`
}

// ErrorNode is a group of child nodes or a leaf describing a generated error metadata field.
type ErrorNode struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Type        string      `json:"type,omitempty"`
	FromReflect bool        `json:"from_reflect,omitempty"`
	Values      []any       `json:"values,omitempty"`
	Modules     []ErrorNode `json:"modules,omitempty"`
}

// DecodeErrorCatalog strictly decodes raw JSON into an ErrorCatalog.
func DecodeErrorCatalog(raw []byte) (ErrorCatalog, error) {
	var catalog ErrorCatalog
	if err := decodeStrict(raw, &catalog); err != nil {
		return ErrorCatalog{}, err
	}
	return catalog, nil
}

// ValidateErrorCatalog reports whether the catalog is well-formed and free of
// duplicate module names, generated type names, meta keys, and option names.
func ValidateErrorCatalog(catalog ErrorCatalog) error {
	if len(catalog.Modules) == 0 {
		return fmt.Errorf("error catalog must define at least one module")
	}

	seenModules := map[string]struct{}{}
	seenTypes := map[string]struct{}{}
	seenMetaKeys := map[string]struct{}{}
	seenOptionNames := map[string]struct{}{}
	seenValueNames := map[string]struct{}{}

	for _, module := range catalog.Modules {
		if err := validateErrorModule(module, seenModules, seenTypes, seenMetaKeys, seenOptionNames, seenValueNames); err != nil {
			return err
		}
	}
	return nil
}

func validateErrorModule(module ErrorModule, seenModules, seenTypes, seenMetaKeys, seenOptionNames, seenValueNames map[string]struct{}) error {
	if module.Name == "" {
		return fmt.Errorf("module name cannot be empty")
	}
	if _, ok := seenModules[module.Name]; ok {
		return fmt.Errorf("duplicate module name %q", module.Name)
	}
	seenModules[module.Name] = struct{}{}

	if len(module.Modules) == 0 {
		return fmt.Errorf("module %q must define at least one child node", module.Name)
	}

	seenChildren := map[string]struct{}{}
	for _, child := range module.Modules {
		if err := validateErrorNode(child, nil, seenChildren, seenTypes, seenMetaKeys, seenOptionNames, seenValueNames); err != nil {
			return err
		}
	}
	return nil
}

func validateErrorNode(node ErrorNode, path []string, seenSiblings, seenTypes, seenMetaKeys, seenOptionNames, seenValueNames map[string]struct{}) error {
	if node.Name == "" {
		return fmt.Errorf("node %q has an empty name", DisplayPath(path))
	}
	if _, ok := seenSiblings[node.Name]; ok {
		return fmt.Errorf("duplicate node name %q within %q", node.Name, DisplayPath(path))
	}
	seenSiblings[node.Name] = struct{}{}

	currentPath := append(append([]string{}, path...), node.Name)
	if len(node.Modules) > 0 {
		if len(node.Values) > 0 {
			return fmt.Errorf("node %q cannot define both values and children", DisplayPath(currentPath))
		}
		if node.Type != "" {
			return fmt.Errorf("group node %q cannot define type", DisplayPath(currentPath))
		}
		if node.FromReflect {
			return fmt.Errorf("group node %q cannot define from_reflect", DisplayPath(currentPath))
		}

		nextSiblings := map[string]struct{}{}
		for _, child := range node.Modules {
			if err := validateErrorNode(child, currentPath, nextSiblings, seenTypes, seenMetaKeys, seenOptionNames, seenValueNames); err != nil {
				return err
			}
		}
		return nil
	}

	typeName := ErrorTypeName(currentPath...)
	metaKey := ErrorMetaKey(currentPath...)
	optionName := ErrorOptionName(typeName)

	if _, ok := seenTypes[typeName]; ok {
		return fmt.Errorf("duplicate generated type name %q", typeName)
	}
	seenTypes[typeName] = struct{}{}

	if _, ok := seenMetaKeys[metaKey]; ok {
		return fmt.Errorf("duplicate meta key %q", metaKey)
	}
	seenMetaKeys[metaKey] = struct{}{}

	if _, ok := seenOptionNames[optionName]; ok {
		return fmt.Errorf("duplicate option name %q", optionName)
	}
	seenOptionNames[optionName] = struct{}{}

	goType := ErrorGoType(node.Type)
	switch goType {
	case "string", "bool", "int", "int64", "float64":
	default:
		return fmt.Errorf("node %q has unsupported type %q", DisplayPath(currentPath), node.Type)
	}
	if node.FromReflect && goType != "string" {
		return fmt.Errorf("node %q can only use from_reflect with type string", DisplayPath(currentPath))
	}

	if len(node.Values) == 0 {
		return nil
	}
	if node.FromReflect {
		return fmt.Errorf("node %q cannot define from_reflect when values are present", DisplayPath(currentPath))
	}

	seenNames := map[string]struct{}{}
	for _, value := range node.Values {
		valueName, _, err := validateAndRenderErrorValue(goType, value)
		if err != nil {
			return fmt.Errorf("node %q value %v: %w", DisplayPath(currentPath), value, err)
		}
		if _, ok := seenNames[valueName]; ok {
			return fmt.Errorf("node %q has duplicate value name %q", DisplayPath(currentPath), valueName)
		}
		seenNames[valueName] = struct{}{}
		if _, ok := seenValueNames[typeName+valueName]; ok {
			return fmt.Errorf("duplicate generated value name %q", typeName+valueName)
		}
		seenValueNames[typeName+valueName] = struct{}{}
	}

	return nil
}

// ErrorTypeName returns the generated Go type name for a node path.
func ErrorTypeName(segments ...string) string {
	return GoNameFromPath(segments...)
}

// ErrorMetaKey returns the underscore-joined metadata key for a node path.
func ErrorMetaKey(segments ...string) string {
	return strings.Join(segments, "_")
}

// ErrorOptionName returns the generated functional-option name for a type name.
func ErrorOptionName(typeName string) string {
	return "With" + typeName
}

// ErrorGoType resolves a node's declared type, defaulting to string when unset.
func ErrorGoType(goType string) string {
	if goType == "" {
		return "string"
	}
	return goType
}

func validateAndRenderErrorValue(goType string, value any) (string, string, error) {
	switch goType {
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", "", fmt.Errorf("expected string value, got %T", value)
		}
		if text == "" {
			return "", "", fmt.Errorf("string value cannot be empty")
		}
		valueName := GoName(text)
		if valueName == "" {
			valueName = "Value"
		}
		return strconv.Quote(text), valueName, nil
	case "bool":
		flag, ok := value.(bool)
		if !ok {
			return "", "", fmt.Errorf("expected bool value, got %T", value)
		}
		if flag {
			return "true", "True", nil
		}
		return "false", "False", nil
	case "int":
		return renderIntegerValue(value, 0)
	case "int64":
		return renderIntegerValue(value, 64)
	case "float64":
		number, ok := value.(json.Number)
		if !ok {
			return "", "", fmt.Errorf("expected JSON number, got %T", value)
		}
		text := number.String()
		if _, err := strconv.ParseFloat(text, 64); err != nil {
			return "", "", fmt.Errorf("invalid float literal %q", text)
		}
		valueName := strings.NewReplacer("-", "Neg", ".", "Point", "+", "", "e", "E").Replace(text)
		switch {
		case valueName == "":
			valueName = "Value"
		case valueName[0] >= '0' && valueName[0] <= '9':
			valueName = "Value" + valueName
		default:
			valueName = GoName(valueName)
		}
		return text, valueName, nil
	default:
		return "", "", fmt.Errorf("unsupported go type %q", goType)
	}
}

func renderIntegerValue(value any, bitSize int) (string, string, error) {
	number, ok := value.(json.Number)
	if !ok {
		return "", "", fmt.Errorf("expected JSON number, got %T", value)
	}
	text := number.String()
	if strings.ContainsAny(text, ".eE") {
		return "", "", fmt.Errorf("expected integer value, got %q", text)
	}
	parsed, err := strconv.ParseInt(text, 10, bitSize)
	if err != nil {
		return "", "", fmt.Errorf("invalid integer literal %q", text)
	}
	literal := strconv.FormatInt(parsed, 10)
	valueName := literal
	if parsed < 0 {
		valueName = "Neg" + strconv.FormatInt(-parsed, 10)
	}
	return literal, valueName, nil
}
