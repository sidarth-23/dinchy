package email

import (
	"context"
	"fmt"
	htmltemplate "html/template"
	"net/url"
	"strings"
	texttemplate "text/template"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

const (
	pathAcceptInvitation = "/accept-invitation"
	pathResetPassword    = "/reset-password"
)

// Mailer renders branded emails from the shared templates and hands them to a
// Sender. It is the single typed entrypoint consumers use; they never assemble
// a raw Message.
type Mailer struct {
	sender  Sender
	html    *htmltemplate.Template
	text    *texttemplate.Template
	baseURL *url.URL
}

// InvitationEmail is the typed input for an organisation invitation email.
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
// used to build call-to-action links. It fails fast when a configured sender
// has no base URL to build links from.
func NewMailer(sender Sender, publicBaseURL string) (*Mailer, error) {
	if sender == nil {
		sender = NoopSender{}
	}
	html, err := htmltemplate.ParseFS(templateFS, "templates/"+htmlLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse HTML email layout: %w", err)
	}
	text, err := texttemplate.ParseFS(templateFS, "templates/"+textLayoutName)
	if err != nil {
		return nil, fmt.Errorf("parse text email layout: %w", err)
	}
	m := &Mailer{sender: sender, html: html, text: text}
	if publicBaseURL != "" {
		base, err := url.Parse(publicBaseURL)
		if err != nil {
			return nil, fmt.Errorf("parse public base URL %q: %w", publicBaseURL, err)
		}
		m.baseURL = base
	} else if sender.Configured() {
		return nil, fmt.Errorf("public base URL is required when email delivery is configured")
	}
	return m, nil
}

// Configured reports whether the underlying sender can deliver mail.
func (m *Mailer) Configured() bool {
	return m.sender.Configured()
}

// SendInvitation renders and sends an organisation invitation email.
func (m *Mailer) SendInvitation(ctx context.Context, data InvitationEmail) error {
	organisation := i18n.P("organisation", data.OrganisationName)
	return m.send(ctx, data.To, presentation{
		Subject:  resolve(i18n.Msg(i18n.CodeEmailInvitationSubject, organisation)),
		Heading:  resolve(i18n.Msg(i18n.CodeEmailInvitationHeading, organisation)),
		Body:     resolve(i18n.Msg(i18n.CodeEmailInvitationBody, organisation, i18n.P("role", data.Role))),
		CTALabel: resolve(i18n.Msg(i18n.CodeEmailInvitationCta)),
		CTAURL:   m.actionURL(pathAcceptInvitation, data.Token),
		Footer:   resolve(i18n.Msg(i18n.CodeEmailFooter)),
	})
}

// SendPasswordReset renders and sends a password reset email.
func (m *Mailer) SendPasswordReset(ctx context.Context, data PasswordResetEmail) error {
	return m.send(ctx, data.To, presentation{
		Subject:  resolve(i18n.Msg(i18n.CodeEmailPasswordResetSubject)),
		Heading:  resolve(i18n.Msg(i18n.CodeEmailPasswordResetHeading)),
		Body:     resolve(i18n.Msg(i18n.CodeEmailPasswordResetBody)),
		CTALabel: resolve(i18n.Msg(i18n.CodeEmailPasswordResetCta)),
		CTAURL:   m.actionURL(pathResetPassword, data.Token),
		Footer:   resolve(i18n.Msg(i18n.CodeEmailFooter)),
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
	if err := m.sender.Send(ctx, Message{To: to, Subject: content.Subject, Text: textBuilder.String(), HTML: htmlBuilder.String()}); err != nil {
		return fmt.Errorf("deliver email to %q: %w", to, err)
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
