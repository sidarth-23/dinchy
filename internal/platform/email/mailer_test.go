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

func TestMailer_Configured(t *testing.T) {
	t.Parallel()

	mailer, err := NewMailer(nil, false)
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}
	if mailer.Configured() {
		t.Fatal("unconfigured mailer must report not configured")
	}
	if err := mailer.Send(context.Background(), "user@example.com", Content{Subject: "Hi"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured when no enqueuer, got %v", err)
	}
}

func TestMailer_SendRendersContentIntoLayoutAndEnqueues(t *testing.T) {
	t.Parallel()

	enqueuer := &captureEnqueuer{}
	mailer, err := NewMailer(enqueuer, true)
	if err != nil {
		t.Fatalf("NewMailer: %v", err)
	}

	content := Content{
		Subject:  "Join Acme on Dinchy",
		Heading:  "Join Acme",
		Body:     "You have been invited to join Acme.",
		CTALabel: "Accept invitation",
		CTAURL:   "https://app.test/accept-invitation?token=invite-token",
		Footer:   "This is an automated message.",
	}
	if err := mailer.Send(context.Background(), "invitee@example.com", content); err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg := enqueuer.only(t)
	if msg.To != "invitee@example.com" {
		t.Errorf("unexpected recipient %q", msg.To)
	}
	if msg.Subject != content.Subject {
		t.Errorf("subject %q should equal content subject %q", msg.Subject, content.Subject)
	}
	if !strings.Contains(msg.Text, content.CTAURL) {
		t.Errorf("plaintext body missing CTA link %q:\n%s", content.CTAURL, msg.Text)
	}
	if !strings.Contains(msg.HTML, content.CTAURL) {
		t.Errorf("HTML body missing CTA link %q", content.CTAURL)
	}
	if !strings.Contains(msg.Text, content.Heading) {
		t.Errorf("plaintext body should render the heading:\n%s", msg.Text)
	}
	if msg.HTML == "" {
		t.Error("HTML body must not be empty")
	}
}
