package email

import (
	"context"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"

	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/jobs"
)

const (
	pathAcceptInvitation = "/accept-invitation"
	pathResetPassword    = "/reset-password"
)

// Mailer renders branded emails from the shared templates and enqueues them for
// durable delivery via the job queue. It is the single typed entrypoint consumers
// use; they never assemble a raw Message and never send inline.
type Mailer struct {
	enqueuer   jobs.Enqueuer
	html       *htmltemplate.Template
	text       *texttemplate.Template
	baseURL    *url.URL
	configured bool
}

// InvitationEmail is the typed input for an organization invitation email.
type InvitationEmail struct {
	To               string
	OrganisationName string
	Role             string
	Token            string
}

// PasswordResetEmail is the typed input for a password reset email.
type PasswordResetEmail struct {
	To    string
	Token string
}

// NewMailer parses the email templates once and validates the public base URL
// used to build call-to-action links. It fails fast when email delivery is
// configured but there is no base URL to build links from. Delivery is durable:
// rendered messages are enqueued through enqueuer and sent by SendEmailWorker.
func NewMailer(enqueuer jobs.Enqueuer, publicBaseURL string, configured bool) (*Mailer, error) {
	html, err := htmltemplate.ParseFS(templateFS, "templates/"+htmlLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse HTML email layout: %w", err)
	}
	text, err := texttemplate.ParseFS(templateFS, "templates/"+textLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse text email layout: %w", err)
	}
	m := &Mailer{enqueuer: enqueuer, html: html, text: text, configured: configured}
	if publicBaseURL != "" {
		base, err := url.Parse(publicBaseURL)
		if err != nil {
			return nil, fmt.Errorf("parse public base URL %q: %w", publicBaseURL, err)
		}
		m.baseURL = base
	} else if configured {
		return nil, fmt.Errorf("public base URL is required when email delivery is configured")
	}
	return m, nil
}

// Configured reports whether email delivery is configured.
func (m *Mailer) Configured() bool {
	return m.configured
}

// SendInvitation renders and sends an organization invitation email.
func (m *Mailer) SendInvitation(ctx context.Context, data InvitationEmail) error {
	organisation := i18n.P("organisation", data.OrganisationName)
	return m.send(ctx, data.To, presentation{
		Subject:  resolve(i18n.Msg(i18n.CodeNotificationEmailInvitationSubject, organisation)),
		Heading:  resolve(i18n.Msg(i18n.CodeNotificationEmailInvitationHeading, organisation)),
		Body:     resolve(i18n.Msg(i18n.CodeNotificationEmailInvitationBody, organisation, i18n.P("role", data.Role))),
		CTALabel: resolve(i18n.Msg(i18n.CodeNotificationEmailInvitationCta)),
		CTAURL:   m.actionURL(pathAcceptInvitation, data.Token),
		Footer:   resolve(i18n.Msg(i18n.CodeNotificationEmailFooter)),
	})
}

// SendPasswordReset renders and sends a password reset email.
func (m *Mailer) SendPasswordReset(ctx context.Context, data PasswordResetEmail) error {
	return m.send(ctx, data.To, presentation{
		Subject:  resolve(i18n.Msg(i18n.CodeNotificationEmailPasswordResetSubject)),
		Heading:  resolve(i18n.Msg(i18n.CodeNotificationEmailPasswordResetHeading)),
		Body:     resolve(i18n.Msg(i18n.CodeNotificationEmailPasswordResetBody)),
		CTALabel: resolve(i18n.Msg(i18n.CodeNotificationEmailPasswordResetCta)),
		CTAURL:   m.actionURL(pathResetPassword, data.Token),
		Footer:   resolve(i18n.Msg(i18n.CodeNotificationEmailFooter)),
	})
}

func (m *Mailer) send(ctx context.Context, to string, content presentation) error {
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

// actionURL builds a call-to-action link. A configured mailer always has a base
// URL (enforced in NewMailer); the nil case only occurs for an unconfigured
// mailer, whose send will fail with ErrNotConfigured before the link matters.
func (m *Mailer) actionURL(path, token string) string {
	link := url.URL{Path: path}
	if m.baseURL != nil {
		link = *m.baseURL
		link.Path = path
	}
	query := link.Query()
	query.Set("token", token)
	link.RawQuery = query.Encode()
	return link.String()
}
