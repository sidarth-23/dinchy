package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// Target identifies the thing an event applies to.
type Target struct {
	targetType    string
	targetID      string
	targetDisplay string
}

// NewTarget constructs a target reference for an event envelope.
func NewTarget(targetType, targetID, targetDisplay string) Target {
	return Target{
		targetType:    strings.TrimSpace(targetType),
		targetID:      strings.TrimSpace(targetID),
		targetDisplay: strings.TrimSpace(targetDisplay),
	}
}

func (target Target) validate() error {
	if strings.TrimSpace(target.targetType) == "" {
		return fmt.Errorf("target type is required")
	}
	if strings.TrimSpace(target.targetID) == "" {
		return fmt.Errorf("target ID is required")
	}
	if strings.TrimSpace(target.targetDisplay) == "" {
		return fmt.Errorf("target display is required")
	}
	return nil
}

// NewEnvelope creates an event envelope and requires a populated target reference.
func NewEnvelope(ctx context.Context, actorUserID, actorOrganisationID string, target Target) (Envelope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := target.validate(); err != nil {
		return Envelope{}, err
	}
	return Envelope{
		ActorUserID:         actorUserID,
		ActorOrganisationID: actorOrganisationID,
		TargetType:          target.targetType,
		TargetID:            target.targetID,
		TargetDisplay:       target.targetDisplay,
		RequestID:           support.RequestIDFrom(ctx),
		TraceID:             support.TraceIDFrom(ctx),
		SpanID:              support.SpanIDFrom(ctx),
		IPAddress:           support.RemoteIPFrom(ctx),
		UserAgent:           support.UserAgentFrom(ctx),
	}, nil
}
