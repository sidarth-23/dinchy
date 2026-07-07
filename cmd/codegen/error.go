package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

type errorCatalog = manifest.ErrorCatalog
type errorModule = manifest.ErrorModule
type errorNode = manifest.ErrorNode

func runError(args []string) error {
	fs := flag.NewFlagSet("error", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	input := fs.String("input", "catalog.json", "manifest input path")
	output := fs.String("output", "generated.go", "generated Go output path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return generateError(*input, *output)
}

func generateError(inputPath, outputPath string) error {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}

	mf, err := manifest.DecodeErrorCatalog(raw)
	if err != nil {
		return err
	}
	if err := manifest.ValidateErrorCatalog(mf); err != nil {
		return err
	}

	src, err := renderErrorCatalog(mf)
	if err != nil {
		return err
	}
	return writeGeneratedFile(outputPath, src)
}

func renderErrorCatalog(mf errorCatalog) ([]byte, error) {
	definitions := flattenErrorDefinitions(mf.Modules)
	sort.SliceStable(definitions, func(i, j int) bool {
		return definitions[i].TypeName < definitions[j].TypeName
	})

	view := errorFileView{Imports: errorImports(definitions)}
	for _, def := range definitions {
		view.MetaKeys = append(view.MetaKeys, errorMetaKeyView{Const: def.MetaKeyConst, Key: def.MetaKey})
		definitionView, err := errorDefinitionViewFor(def)
		if err != nil {
			return nil, err
		}
		view.Definitions = append(view.Definitions, definitionView)
	}
	return renderTemplate("error.go.tmpl", view)
}

type errorFileView struct {
	Imports     []string
	MetaKeys    []errorMetaKeyView
	Definitions []errorDefinitionView
}

type errorMetaKeyView struct {
	Const string
	Key   string
}

type errorDefinitionView struct {
	Description  string
	TypeName     string
	GoType       string
	FromReflect  bool
	Receiver     string
	MetaKeyConst string
	OptionName   string
	Values       []errorValueView
}

type errorValueView struct {
	Suffix  string
	Literal string
}

func errorDefinitionViewFor(def flattenedErrorDefinition) (errorDefinitionView, error) {
	view := errorDefinitionView{
		Description:  def.Description,
		TypeName:     def.TypeName,
		GoType:       def.GoType,
		FromReflect:  def.FromReflect,
		Receiver:     receiverName(def.TypeName),
		MetaKeyConst: def.MetaKeyConst,
		OptionName:   def.OptionName,
	}
	for _, value := range def.Values {
		literal, suffix, err := renderErrorValue(def.GoType, value)
		if err != nil {
			return errorDefinitionView{}, fmt.Errorf("render constants for %s: %w", def.TypeName, err)
		}
		view.Values = append(view.Values, errorValueView{Suffix: suffix, Literal: literal})
	}
	return view, nil
}

type flattenedErrorDefinition struct {
	TypeName     string
	MetaKey      string
	MetaKeyConst string
	OptionName   string
	Description  string
	GoType       string
	FromReflect  bool
	Values       []any
}

func flattenErrorDefinitions(modules []errorModule) []flattenedErrorDefinition {
	out := make([]flattenedErrorDefinition, 0)
	for _, module := range modules {
		for _, node := range module.Modules {
			out = append(out, flattenErrorDefinitionsForNode([]string{}, node)...)
		}
	}
	return out
}

func flattenErrorDefinitionsForNode(path []string, node errorNode) []flattenedErrorDefinition {
	currentPath := append(append([]string{}, path...), node.Name)
	if len(node.Modules) > 0 {
		out := make([]flattenedErrorDefinition, 0)
		for _, child := range node.Modules {
			out = append(out, flattenErrorDefinitionsForNode(currentPath, child)...)
		}
		return out
	}

	goType := manifest.ErrorGoType(node.Type)
	def := flattenedErrorDefinition{
		TypeName:     manifest.ErrorTypeName(currentPath...),
		MetaKey:      manifest.ErrorMetaKey(currentPath...),
		MetaKeyConst: errorMetaKeyConstName(currentPath...),
		OptionName:   manifest.ErrorOptionName(manifest.ErrorTypeName(currentPath...)),
		Description:  node.Description,
		GoType:       goType,
		FromReflect:  node.FromReflect,
		Values:       normalizeErrorValues(node.Values),
	}
	return []flattenedErrorDefinition{def}
}

func errorImports(definitions []flattenedErrorDefinition) []string {
	imports := map[string]struct{}{}
	for _, def := range definitions {
		if def.FromReflect {
			imports["reflect"] = struct{}{}
		}
	}
	if len(imports) == 0 {
		return nil
	}
	out := make([]string, 0, len(imports))
	for imp := range imports {
		out = append(out, imp)
	}
	sort.Strings(out)
	return out
}

func normalizeErrorValues(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	out := make([]any, len(values))
	copy(out, values)
	return out
}

func errorMetaKeyConstName(segments ...string) string {
	return "MetaKey" + manifest.ErrorTypeName(segments...)
}

func renderErrorValue(goType string, value any) (string, string, error) {
	switch goType {
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", "", fmt.Errorf("expected string value, got %T", value)
		}
		if text == "" {
			return "", "", fmt.Errorf("string value cannot be empty")
		}
		return strconv.Quote(text), errorValueNameForString(text), nil
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
		return renderFloatValue(value)
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
	return strconv.FormatInt(parsed, 10), errorValueNameForNumber(parsed), nil
}

func renderFloatValue(value any) (string, string, error) {
	number, ok := value.(json.Number)
	if !ok {
		return "", "", fmt.Errorf("expected JSON number, got %T", value)
	}
	text := number.String()
	if _, err := strconv.ParseFloat(text, 64); err != nil {
		return "", "", fmt.Errorf("invalid float literal %q", text)
	}
	return text, errorValueNameForFloat(text), nil
}

func errorValueNameForString(value string) string {
	name := manifest.GoName(value)
	if name == "" {
		return "Value"
	}
	return name
}

func errorValueNameForNumber(value int64) string {
	if value < 0 {
		return "Neg" + strconv.FormatInt(-value, 10)
	}
	return strconv.FormatInt(value, 10)
}

func errorValueNameForFloat(value string) string {
	name := strings.NewReplacer("-", "Neg", ".", "Point", "+", "", "e", "E").Replace(value)
	if name == "" {
		return "Value"
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "Value" + name
	}
	return manifest.GoName(name)
}

func receiverName(typeName string) string {
	if typeName == "" {
		return "value"
	}
	runes := []rune(typeName)
	boundary := 1
	for i := 1; i < len(runes); i++ {
		if isReceiverBoundary(runes, i) {
			boundary = i
			break
		}
	}
	for i := 0; i < boundary; i++ {
		runes[i] = unicodeLower(runes[i])
	}
	return string(runes)
}

func unicodeLower(r rune) rune {
	if 'A' <= r && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

func isReceiverBoundary(runes []rune, index int) bool {
	current := runes[index]
	previous := runes[index-1]
	next := rune(0)
	if index+1 < len(runes) {
		next = runes[index+1]
	}
	if 'A' <= current && current <= 'Z' {
		if ('a' <= previous && previous <= 'z') || ('0' <= previous && previous <= '9') {
			return true
		}
		if 'A' <= previous && previous <= 'Z' && ('a' <= next && next <= 'z') {
			return true
		}
	}
	return false
}
