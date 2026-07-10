package events

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

type testSubscriber struct{}

func (testSubscriber) Name() string {
	return "audit"
}

func (testSubscriber) Logger(context.Context) *slog.Logger {
	return slog.Default()
}

func (testSubscriber) Handle(context.Context, Record) error {
	return nil
}

func TestNewWorkerDerivesSubscriberName(t *testing.T) {
	worker := NewWorker(nil, testSubscriber{})

	assert.Equal(t, "eventbus_audit", worker.TaskName())
	assert.Equal(t, "eventbus.audit.failed", worker.FailureErrorCode())
}
