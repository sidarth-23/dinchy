// Package main implements the codegen binary that renders manifest catalogs into generated Go source.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

type (
	errorCatalog = manifest.ErrorCatalog
	errorModule  = manifest.ErrorModule
	errorNode    = manifest.ErrorNode
)

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
		Receiver:     manifest.LowerCamel(def.TypeName),
		MetaKeyConst: def.MetaKeyConst,
		OptionName:   def.OptionName,
	}
	for _, value := range def.Values {
		rendered, err := manifest.RenderErrorValue(def.GoType, value)
		if err != nil {
			return errorDefinitionView{}, fmt.Errorf("render constants for %s: %w", def.TypeName, err)
		}
		view.Values = append(view.Values, errorValueView{Suffix: rendered.Suffix, Literal: rendered.Literal})
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
		Values:       slices.Clone(node.Values),
	}
	return []flattenedErrorDefinition{def}
}

func errorImports(definitions []flattenedErrorDefinition) []string {
	for _, def := range definitions {
		if def.FromReflect {
			return []string{"reflect"}
		}
	}
	return nil
}

func errorMetaKeyConstName(segments ...string) string {
	return "MetaKey" + manifest.ErrorTypeName(segments...)
}
