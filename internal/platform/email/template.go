package email

import "embed"

//go:embed templates/*.tmpl
var templateFS embed.FS

const (
	htmlLayoutName = "layout.html.tmpl"
	textLayoutName = "layout.txt.tmpl"
)
