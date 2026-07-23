package email

import (
	"context"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"

	"github.com/sidarth-23/dinchy/internal/platform/jobs"
)

// Mailer renders resolved Content into the shared branded layout and enqueues it
// for durable delivery via the job queue. It is the single delivery entrypoint:
// it selects no copy and builds no links, so callers supply a fully-resolved
// Content and never assemble a raw Message or send inline.
type Mailer struct {
	enqueuer   jobs.Enqueuer
	html       *htmltemplate.Template
	text       *texttemplate.Template
	configured bool
}

// Content is a fully-resolved email ready to render into the shared layout. Copy
// selection, localization, and link building are the caller's responsibility.
type Content struct {
	Subject  string
	Heading  string
	Body     string
	CTALabel string
	CTAURL   string
	Footer   string
}

// NewMailer parses the shared email layouts once. Delivery is durable: rendered
// messages are enqueued through enqueuer and sent by SendEmailWorker. The
// configured flag reports whether email delivery is available to callers.
func NewMailer(enqueuer jobs.Enqueuer, configured bool) (*Mailer, error) {
	html, err := htmltemplate.ParseFS(templateFS, "templates/"+htmlLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse HTML email layout: %w", err)
	}
	text, err := texttemplate.ParseFS(templateFS, "templates/"+textLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse text email layout: %w", err)
	}
	return &Mailer{enqueuer: enqueuer, html: html, text: text, configured: configured}, nil
}

// Configured reports whether email delivery is configured.
func (m *Mailer) Configured() bool {
	return m.configured
}

// Send renders content into the shared layout and enqueues it for durable delivery.
func (m *Mailer) Send(ctx context.Context, to string, content Content) error {
	var textBuilder strings.Builder
	if err := m.text.ExecuteTemplate(&textBuilder, textLayoutName, content); err != nil {
		return fmt.Errorf("render text body for subject %q: %w", content.Subject, err)
	}
	var htmlBuilder strings.Builder
	if err := m.html.ExecuteTemplate(&htmlBuilder, htmlLayoutName, content); err != nil {
		return fmt.Errorf("render HTML body for subject %q: %w", content.Subject, err)
	}
	if m.enqueuer == nil {
		return fmt.Errorf("enqueue email to %q: %w", to, ErrNotConfigured)
	}
	args := SendEmailArgs{To: to, Subject: content.Subject, Text: textBuilder.String(), HTML: htmlBuilder.String()}
	if err := m.enqueuer.Enqueue(ctx, args, nil); err != nil {
		return fmt.Errorf("enqueue email to %q: %w", to, err)
	}
	return nil
}
