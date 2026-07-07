package main

import (
	"embed"
	"fmt"
	"go/format"
	"strconv"
	"strings"
	"text/template"
)

//go:embed templates/*.go.tmpl
var templateFS embed.FS

// templates holds every generator template parsed once with the shared helper
// funcs. Each generator selects its template by base filename.
var templates = template.Must(
	template.New("codegen").Funcs(templateFuncs).ParseFS(templateFS, "templates/*.go.tmpl"),
)

// templateFuncs stays intentionally small: view models carry every derived
// value, so the templates only need to quote raw literals.
var templateFuncs = template.FuncMap{
	"quote": strconv.Quote,
}

// renderTemplate executes the named template into gofmt-formatted Go source. It
// owns the format step so callers stay focused on building their view model.
func renderTemplate(templateName string, data any) ([]byte, error) {
	var b strings.Builder
	if err := templates.ExecuteTemplate(&b, templateName, data); err != nil {
		return nil, fmt.Errorf("execute template %q: %w", templateName, err)
	}
	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return nil, fmt.Errorf("format generated source from template %q: %w", templateName, err)
	}
	return formatted, nil
}
