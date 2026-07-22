package email

import (
	"context"
	"testing"

	"github.com/riverqueue/river"
)

type captureSender struct {
	sent []Message
}

func (c *captureSender) Configured() bool { return true }

func (c *captureSender) Send(_ context.Context, msg Message) error {
	c.sent = append(c.sent, msg)
	return nil
}

func TestSendEmailWorker_Work_DeliversMessage(t *testing.T) {
	t.Parallel()

	sender := &captureSender{}
	worker := NewSendEmailWorker(sender)

	err := worker.Work(context.Background(), &river.Job[SendEmailArgs]{
		Args: SendEmailArgs{To: "user@example.com", Subject: "Hi", Text: "text body", HTML: "<p>html body</p>"},
	})
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sender.sent))
	}
	msg := sender.sent[0]
	if msg.To != "user@example.com" || msg.Subject != "Hi" || msg.Text != "text body" || msg.HTML != "<p>html body</p>" {
		t.Errorf("worker delivered unexpected message: %+v", msg)
	}
}
