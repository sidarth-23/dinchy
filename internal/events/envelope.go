package events

import (
	"context"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// NewEnvelope creates an event envelope and fills request-scoped observability fields from ctx.
func NewEnvelope(ctx context.Context, actorUserID, actorOrganisationID, targetType, targetID, targetDisplay string) Envelope {
	if ctx == nil {
		ctx = context.Background()
	}
	return Envelope{
		ActorUserID:         actorUserID,
		ActorOrganisationID: actorOrganisationID,
		TargetType:          targetType,
		TargetID:            targetID,
		TargetDisplay:       targetDisplay,
		RequestID:           support.RequestIDFrom(ctx),
		TraceID:             support.TraceIDFrom(ctx),
		SpanID:              support.SpanIDFrom(ctx),
		IPAddress:           support.RemoteIPFrom(ctx),
		UserAgent:           support.UserAgentFrom(ctx),
	}
}
