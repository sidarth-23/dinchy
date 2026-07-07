package email

import (
	"context"
	"fmt"
	"net/url"
)

const (
	pathAcceptInvitation = "/accept-invitation"
	pathResetPassword    = "/reset-password"
)

// Mailer renders branded emails from the shared templates and hands them to a
// Sender. It is the single typed entrypoint consumers use; they never assemble
// a raw Message.
type Mailer struct {
	sender   Sender
	renderer *renderer
	baseURL  *url.URL
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
	renderer, err := newRenderer()
	if err != nil {
		return nil, err
	}
	m := &Mailer{sender: sender, renderer: renderer}
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
	return m.send(ctx, data.To, invitationContent(data, m.actionURL(pathAcceptInvitation, data.Token)))
}

// SendPasswordReset renders and sends a password reset email.
func (m *Mailer) SendPasswordReset(ctx context.Context, data PasswordResetEmail) error {
	return m.send(ctx, data.To, passwordResetContent(m.actionURL(pathResetPassword, data.Token)))
}

func (m *Mailer) send(ctx context.Context, to string, content presentation) error {
	text, html, err := m.renderer.render(content)
	if err != nil {
		return err
	}
	if err := m.sender.Send(ctx, Message{To: to, Subject: content.Subject, Text: text, HTML: html}); err != nil {
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
