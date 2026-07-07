package email

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type captureSender struct {
	configured bool
	sent       []Message
}

func (c *captureSender) Configured() bool { return c.configured }

func (c *captureSender) Send(_ context.Context, msg Message) error {
	c.sent = append(c.sent, msg)
	return nil
}

func TestNewMailer_RequiresBaseURLWhenConfigured(t *testing.T) {
	t.Parallel()

	if _, err := NewMailer(&captureSender{configured: true}, ""); err == nil {
		t.Fatal("expected error when a configured sender has no public base URL")
	}
	if _, err := NewMailer(NoopSender{}, ""); err != nil {
		t.Fatalf("noop sender should not require a base URL: %v", err)
	}
}

func TestMailer_Configured(t *testing.T) {
	t.Parallel()

	mailer, err := NewMailer(NoopSender{}, "")
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}
	if mailer.Configured() {
		t.Fatal("noop-backed mailer must report not configured")
	}
	if err := mailer.SendPasswordReset(context.Background(), PasswordResetEmail{To: "user@example.com", Token: "tok"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestMailer_SendInvitation(t *testing.T) {
	t.Parallel()

	sender := &captureSender{configured: true}
	mailer, err := NewMailer(sender, "https://app.test")
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}

	err = mailer.SendInvitation(context.Background(), InvitationEmail{
		To:               "invitee@example.com",
		OrganisationName: "Acme",
		Role:             "member",
		Token:            "invite-token",
	})
	if err != nil {
		t.Fatalf("SendInvitation: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.sent))
	}
	msg := sender.sent[0]
	if msg.To != "invitee@example.com" {
		t.Errorf("unexpected recipient %q", msg.To)
	}
	if !strings.Contains(msg.Subject, "Acme") {
		t.Errorf("subject should carry the organization name, got %q", msg.Subject)
	}
	wantLink := "https://app.test/accept-invitation?token=invite-token"
	if !strings.Contains(msg.Text, wantLink) {
		t.Errorf("plaintext body missing CTA link %q:\n%s", wantLink, msg.Text)
	}
	if !strings.Contains(msg.HTML, wantLink) {
		t.Errorf("HTML body missing CTA link %q", wantLink)
	}
	if !strings.Contains(msg.Text, "Acme") {
		t.Errorf("plaintext body should mention the organization:\n%s", msg.Text)
	}
}

func TestMailer_SendPasswordReset(t *testing.T) {
	t.Parallel()

	sender := &captureSender{configured: true}
	mailer, err := NewMailer(sender, "https://app.test")
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}

	if err := mailer.SendPasswordReset(context.Background(), PasswordResetEmail{To: "user@example.com", Token: "reset-token"}); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message, got %d", len(sender.sent))
	}
	msg := sender.sent[0]
	wantLink := "https://app.test/reset-password?token=reset-token"
	if !strings.Contains(msg.Text, wantLink) {
		t.Errorf("plaintext body missing CTA link %q:\n%s", wantLink, msg.Text)
	}
	if !strings.Contains(msg.HTML, wantLink) {
		t.Errorf("HTML body missing CTA link %q", wantLink)
	}
	if msg.HTML == "" {
		t.Error("HTML body must not be empty")
	}
}
