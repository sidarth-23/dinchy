package email

import (
	"embed"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

const (
	htmlLayoutName = "layout.html.tmpl"
	textLayoutName = "layout.txt.tmpl"
)

// renderer holds the parsed HTML and plaintext layouts shared by every email.
type renderer struct {
	html *htmltemplate.Template
	text *texttemplate.Template
}

func newRenderer() (*renderer, error) {
	html, err := htmltemplate.ParseFS(templateFS, "templates/"+htmlLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse HTML email layout: %w", err)
	}
	text, err := texttemplate.ParseFS(templateFS, "templates/"+textLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse text email layout: %w", err)
	}
	return &renderer{html: html, text: text}, nil
}

// render produces the plaintext and HTML bodies for a resolved presentation.
func (r *renderer) render(content presentation) (text string, html string, err error) {
	var textBuilder strings.Builder
	if err := r.text.ExecuteTemplate(&textBuilder, textLayoutName, content); err != nil {
		return "", "", fmt.Errorf("render text body for subject %q: %w", content.Subject, err)
	}
	var htmlBuilder strings.Builder
	if err := r.html.ExecuteTemplate(&htmlBuilder, htmlLayoutName, content); err != nil {
		return "", "", fmt.Errorf("render HTML body for subject %q: %w", content.Subject, err)
	}
	return textBuilder.String(), htmlBuilder.String(), nil
}
