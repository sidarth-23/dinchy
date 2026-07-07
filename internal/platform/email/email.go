package email

import (
	"context"
	"fmt"

	"github.com/wneessen/go-mail"

	"github.com/sidarth-23/dinchy/internal/config"
)

// Message is a single transactional email with optional HTML alternative.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers transactional email. Send returns ErrNotConfigured when no
// transport is available, and Configured reports whether Send can deliver.
type Sender interface {
	Send(ctx context.Context, msg Message) error
	Configured() bool
}

// NoopSender is a Sender that delivers nothing, for when SMTP is disabled.
type NoopSender struct{}

// Send always fails with ErrNotConfigured.
func (NoopSender) Send(context.Context, Message) error {
	return ErrNotConfigured
}

// Configured always reports false.
func (NoopSender) Configured() bool {
	return false
}

// ErrNotConfigured indicates SMTP is not configured and email cannot be sent.
var ErrNotConfigured = fmt.Errorf("smtp is not configured")

// SMTPSender delivers email over SMTP using the configured transport.
type SMTPSender struct {
	cfg config.SMTPConfig
}

// NewSMTPSender builds an SMTPSender from cfg, returning ErrNotConfigured when
// SMTP is disabled and an error when required host or from fields are missing.
func NewSMTPSender(cfg config.SMTPConfig) (*SMTPSender, error) {
	if !cfg.Enabled() {
		return nil, ErrNotConfigured
	}
	if cfg.Host == "" || cfg.From == "" {
		return nil, fmt.Errorf("DINCHY_SMTP_HOST and DINCHY_SMTP_FROM are required when SMTP is configured")
	}
	return &SMTPSender{cfg: cfg}, nil
}

// Configured always reports true.
func (s *SMTPSender) Configured() bool {
	return true
}

// Send composes msg and delivers it over SMTP using the sender's configuration.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	m := mail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return fmt.Errorf("set email from address %q: %w", s.cfg.From, err)
	}
	if err := m.To(msg.To); err != nil {
		return fmt.Errorf("set email recipient %q: %w", msg.To, err)
	}
	m.Subject(msg.Subject)
	m.SetBodyString(mail.TypeTextPlain, msg.Text)
	if msg.HTML != "" {
		m.AddAlternativeString(mail.TypeTextHTML, msg.HTML)
	}

	options := []mail.Option{mail.WithPort(int(s.cfg.Port))}
	if s.cfg.Username != "" {
		options = append(options, mail.WithSMTPAuth(mail.SMTPAuthPlain), mail.WithUsername(s.cfg.Username), mail.WithPassword(s.cfg.Password))
	}
	client, err := mail.NewClient(s.cfg.Host, options...)
	if err != nil {
		return fmt.Errorf("create smtp client for host %q: %w", s.cfg.Host, err)
	}
	if err := client.DialAndSendWithContext(ctx, m); err != nil {
		return fmt.Errorf("send email to %q: %w", msg.To, err)
	}
	return nil
}
