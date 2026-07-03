package email

import (
	"context"
	"fmt"
	"strconv"

	"github.com/wneessen/go-mail"

	"github.com/sidarth-23/dinchy/internal/config"
)

type Message struct {
	To      string
	Subject string
	Text    string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
	Configured() bool
}

type NoopSender struct{}

func (NoopSender) Send(context.Context, Message) error {
	return ErrNotConfigured
}

func (NoopSender) Configured() bool {
	return false
}

var ErrNotConfigured = fmt.Errorf("smtp is not configured")

type SMTPSender struct {
	cfg config.SMTPConfig
}

func NewSMTPSender(cfg config.SMTPConfig) (*SMTPSender, error) {
	if !cfg.Enabled() {
		return nil, ErrNotConfigured
	}
	if cfg.Host == "" || cfg.From == "" {
		return nil, fmt.Errorf("DINCHY_SMTP_HOST and DINCHY_SMTP_FROM are required when SMTP is configured")
	}
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return nil, fmt.Errorf("parse DINCHY_SMTP_PORT %q: %w", cfg.Port, err)
	}
	return &SMTPSender{cfg: cfg}, nil
}

func (s *SMTPSender) Configured() bool {
	return true
}

func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	port, err := strconv.Atoi(s.cfg.Port)
	if err != nil {
		return fmt.Errorf("parse smtp port %q: %w", s.cfg.Port, err)
	}
	m := mail.NewMsg()
	if err := m.From(s.cfg.From); err != nil {
		return fmt.Errorf("set email from address %q: %w", s.cfg.From, err)
	}
	if err := m.To(msg.To); err != nil {
		return fmt.Errorf("set email recipient %q: %w", msg.To, err)
	}
	m.Subject(msg.Subject)
	m.SetBodyString(mail.TypeTextPlain, msg.Text)

	options := []mail.Option{mail.WithPort(port)}
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
