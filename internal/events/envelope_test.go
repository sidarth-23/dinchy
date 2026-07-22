package events_test

import (
	"context"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/platform/requestcontext"
)

func TestNewEnvelope_PopulatesRequestAndTraceMetadata(t *testing.T) {
	ctx := context.WithValue(context.Background(), chimw.RequestIDKey, "req-123")
	ctx = requestcontext.WithRequestInfo(ctx, "203.0.113.5", "Mozilla/5.0")
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		TraceFlags: trace.FlagsSampled,
	}))

	envelope, err := events.NewEnvelope(ctx, "user-1", "org-1", events.NewTarget("session", "session-1", "display"))
	require.NoError(t, err)

	assert.Equal(t, "user-1", envelope.ActorUserID)
	assert.Equal(t, "org-1", envelope.ActorOrganizationID)
	assert.Equal(t, "session", envelope.TargetType)
	assert.Equal(t, "session-1", envelope.TargetID)
	assert.Equal(t, "display", envelope.TargetDisplay)
	assert.Equal(t, "req-123", envelope.RequestID)
	assert.Equal(t, "0102030405060708090a0b0c0d0e0f10", envelope.TraceID)
	assert.Equal(t, "1112131415161718", envelope.SpanID)
	assert.Equal(t, "203.0.113.5", envelope.IPAddress)
	assert.Equal(t, "Mozilla/5.0", envelope.UserAgent)
}

func TestNewEnvelope_RequiresTargetFields(t *testing.T) {
	t.Parallel()

	_, err := events.NewEnvelope(context.Background(), "user-1", "org-1", events.NewTarget("", "session-1", "display"))
	require.Error(t, err)

	_, err = events.NewEnvelope(context.Background(), "user-1", "org-1", events.NewTarget("session", "", "display"))
	require.Error(t, err)

	_, err = events.NewEnvelope(context.Background(), "user-1", "org-1", events.NewTarget("session", "session-1", ""))
	require.Error(t, err)
}
