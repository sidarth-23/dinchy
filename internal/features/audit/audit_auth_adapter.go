package audit

import (
	"context"

	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

type AuthRecorder struct {
	service *Service
}

func NewAuthRecorder(service *Service) AuthRecorder {
	return AuthRecorder{service: service}
}

func (r AuthRecorder) RecordAuthEvent(ctx context.Context, event auth.AuditEvent) error {
	if r.service == nil || !r.service.Enabled() {
		return nil
	}
	session := auth.SessionFrom(ctx)
	if event.ActorUserID == "" && session != nil {
		event.ActorUserID = session.UserID
	}
	if event.ActorOrganisationID == "" && session != nil {
		event.ActorOrganisationID = session.OrganisationID
	}
	if event.IPAddress == "" {
		event.IPAddress = support.RemoteIPFrom(ctx)
	}
	if event.UserAgent == "" {
		event.UserAgent = support.UserAgentFrom(ctx)
	}
	return r.service.Record(ctx, Event{
		Category:            event.Category,
		Subcategory:         event.Subcategory,
		EventType:           event.EventType,
		Action:              event.Action,
		Outcome:             event.Outcome,
		ActorUserID:         event.ActorUserID,
		ActorOrganisationID: event.ActorOrganisationID,
		TargetType:          event.TargetType,
		TargetID:            event.TargetID,
		TargetDisplay:       event.TargetDisplay,
		RequestID:           support.RequestIDFrom(ctx),
		TraceID:             support.TraceIDFrom(ctx),
		SpanID:              support.SpanIDFrom(ctx),
		IPAddress:           event.IPAddress,
		UserAgent:           event.UserAgent,
		Metadata:            event.Metadata,
		Changes:             event.Changes,
	})
}
