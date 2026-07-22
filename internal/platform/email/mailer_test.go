package email

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
)

type captureEnqueuer struct {
	enqueued []river.JobArgs
}

func (c *captureEnqueuer) Enqueue(_ context.Context, args river.JobArgs, _ *river.InsertOpts) error {
	c.enqueued = append(c.enqueued, args)
	return nil
}

func (c *captureEnqueuer) EnqueueTx(_ context.Context, _ pgx.Tx, args river.JobArgs, _ *river.InsertOpts) error {
	c.enqueued = append(c.enqueued, args)
	return nil
}

func (c *captureEnqueuer) only(t *testing.T) SendEmailArgs {
	t.Helper()
	if len(c.enqueued) != 1 {
		t.Fatalf("expected 1 enqueued email, got %d", len(c.enqueued))
	}
	args, ok := c.enqueued[0].(SendEmailArgs)
	if !ok {
		t.Fatalf("enqueued job is %T, want SendEmailArgs", c.enqueued[0])
	}
	return args
}

func TestNewMailer_RequiresBaseURLWhenConfigured(t *testing.T) {
	t.Parallel()

	if _, err := NewMailer(&captureEnqueuer{}, "", true); err == nil {
		t.Fatal("expected error when email is configured but has no public base URL")
	}
	if _, err := NewMailer(&captureEnqueuer{}, "", false); err != nil {
		t.Fatalf("unconfigured mailer should not require a base URL: %v", err)
	}
}

func TestMailer_Configured(t *testing.T) {
	t.Parallel()

	mailer, err := NewMailer(nil, "", false)
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}
	if mailer.Configured() {
		t.Fatal("unconfigured mailer must report not configured")
	}
	if err := mailer.SendPasswordReset(context.Background(), PasswordResetEmail{To: "user@example.com", Token: "tok"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}

func TestMailer_SendInvitation(t *testing.T) {
	t.Parallel()

	enqueuer := &captureEnqueuer{}
	mailer, err := NewMailer(enqueuer, "https://app.test", true)
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
	msg := enqueuer.only(t)
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

	enqueuer := &captureEnqueuer{}
	mailer, err := NewMailer(enqueuer, "https://app.test", true)
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}

	if err := mailer.SendPasswordReset(context.Background(), PasswordResetEmail{To: "user@example.com", Token: "reset-token"}); err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}
	msg := enqueuer.only(t)
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
