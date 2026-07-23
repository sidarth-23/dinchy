package email

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"
)

// SendEmailArgs is the durable job payload for delivering one rendered email.
type SendEmailArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

// Kind returns the durable job type identifier.
func (SendEmailArgs) Kind() string { return "email.send" }

// SendEmailWorker delivers a rendered email through the sender as a durable job
// that the queue retries until it succeeds.
type SendEmailWorker struct {
	river.WorkerDefaults[SendEmailArgs]
	sender Sender
}

// NewSendEmailWorker builds the worker that delivers enqueued emails.
func NewSendEmailWorker(sender Sender) *SendEmailWorker {
	if sender == nil {
		sender = NoopSender{}
	}
	return &SendEmailWorker{sender: sender}
}

// Work delivers the email; a returned error is retried by the queue.
func (w *SendEmailWorker) Work(ctx context.Context, job *river.Job[SendEmailArgs]) error {
	msg := Message{To: job.Args.To, Subject: job.Args.Subject, Text: job.Args.Text, HTML: job.Args.HTML}
	if err := w.sender.Send(ctx, msg); err != nil {
		return fmt.Errorf("deliver email to %q: %w", msg.To, err)
	}
	return nil
}
