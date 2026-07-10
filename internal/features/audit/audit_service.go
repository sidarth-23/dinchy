package audit

import (
	"context"
	"fmt"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// Service records and lists audit events. It implements events.Subscriber.
type Service struct {
	features.BaseService
	store Store
}

// Dependencies contains the dependencies required by the audit Service.
type Dependencies struct {
	Base  features.ServiceDependencies
	Store Store
}

// NewService builds an audit Service, requiring a non-nil store and clock.
func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Store == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("audit store is required")))
	}
	base, err := features.NewBaseService("audit", dependencies.Base)
	if err != nil {
		return nil, apperrors.Annotate(err)
	}
	return &Service{BaseService: base, store: dependencies.Store}, nil
}

// Handle persists an event as an audit log, assigning an ID and timestamp when absent.
func (s *Service) Handle(ctx context.Context, event events.Record) error {
	if event.ID == "" {
		event.ID = s.IDGenerator().New()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = s.Clock().Now()
	}
	params, err := insertParams(event)
	if err != nil {
		return err
	}
	if err := s.store.InsertAuditLog(ctx, params); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}

var (
	_ events.Subscriber = (*Service)(nil)
	_ features.Service  = (*Service)(nil)
)

// List returns audit records matching the given filters.
func (s *Service) List(ctx context.Context, in ListInput) ([]events.Record, error) {
	actorUserID, err := id.NullUUID(in.ActorUserID, in.ActorUserID != "")
	if err != nil {
		return nil, apperrors.UnprocessableEntity(i18n.Msg(i18n.CodeRequestValidationFailed),
			apperrors.WithFieldName(apperrors.FieldName("actor_user_id")),
			apperrors.WithCause(err))
	}
	rows, err := s.store.ListAuditLogs(ctx, sqlcgen.ListAuditLogsParams{
		CategoryFilter:    in.Category,
		Category:          in.Category,
		SubcategoryFilter: in.Subcategory,
		Subcategory:       in.Subcategory,
		EventTypeFilter:   in.EventType,
		EventType:         in.EventType,
		ActorUserIDFilter: in.ActorUserID,
		ActorUserID:       actorUserID,
		TargetTypeFilter:  in.TargetType,
		TargetType:        sqltype.Text(in.TargetType),
		TargetIDFilter:    in.TargetID,
		TargetID:          sqltype.Text(in.TargetID),
		OutcomeFilter:     in.Outcome,
		Outcome:           in.Outcome,
		Limit:             int32(in.Limit),
		Before:            sqltype.OptionalTimestamptz(in.Before, in.BeforeValid),
	})
	if err != nil {
		return nil, err
	}
	out := make([]events.Record, 0, len(rows))
	for _, row := range rows {
		log, err := logFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, nil
}

func insertParams(event events.Record) (sqlcgen.InsertAuditLogParams, error) {
	metadataJSON, err := marshalMap("audit metadata", event.EventType, event.Metadata)
	if err != nil {
		return sqlcgen.InsertAuditLogParams{}, err
	}
	changesJSON, err := marshalMap("audit changes", event.EventType, event.Changes)
	if err != nil {
		return sqlcgen.InsertAuditLogParams{}, err
	}
	eventID, err := id.Parse(event.ID)
	if err != nil {
		return sqlcgen.InsertAuditLogParams{}, fmt.Errorf("parse audit event ID %q for event type %q: %w", event.ID, event.EventType, err)
	}
	actorUserID, err := id.NullUUID(event.ActorUserID, event.ActorUserID != "")
	if err != nil {
		return sqlcgen.InsertAuditLogParams{}, fmt.Errorf("parse audit actor user ID %q for event type %q: %w", event.ActorUserID, event.EventType, err)
	}
	actorOrganisationID, err := id.NullUUID(event.ActorOrganisationID, event.ActorOrganisationID != "")
	if err != nil {
		return sqlcgen.InsertAuditLogParams{}, fmt.Errorf("parse audit actor organization ID %q for event type %q: %w", event.ActorOrganisationID, event.EventType, err)
	}
	return sqlcgen.InsertAuditLogParams{
		ID:                  eventID,
		Category:            event.Category,
		Subcategory:         event.Subcategory,
		EventType:           event.EventType,
		Action:              event.Action,
		Outcome:             event.Outcome,
		ActorUserID:         actorUserID,
		ActorOrganisationID: actorOrganisationID,
		TargetType:          sqltype.Text(event.TargetType),
		TargetID:            sqltype.Text(event.TargetID),
		TargetDisplay:       sqltype.Text(event.TargetDisplay),
		RequestID:           sqltype.Text(event.RequestID),
		TraceID:             sqltype.Text(event.TraceID),
		SpanID:              sqltype.Text(event.SpanID),
		IpAddress:           event.IPAddress,
		UserAgent:           event.UserAgent,
		MetadataJson:        metadataJSON,
		ChangesJson:         changesJSON,
		CreatedAt:           sqltype.Timestamptz(event.CreatedAt),
	}, nil
}

func logFromRow(row sqlcgen.AppAuditLog) (events.Record, error) {
	metadata, err := unmarshalMap("audit metadata", row.EventType, row.MetadataJson)
	if err != nil {
		return events.Record{}, err
	}
	changes, err := unmarshalMap("audit changes", row.EventType, row.ChangesJson)
	if err != nil {
		return events.Record{}, err
	}
	return events.Record{
		ID: row.ID.String(), Category: row.Category, Subcategory: row.Subcategory, EventType: row.EventType,
		Action: row.Action, Outcome: row.Outcome, ActorUserID: id.NullUUIDString(row.ActorUserID),
		ActorOrganisationID: id.NullUUIDString(row.ActorOrganisationID),
		TargetType:          sqltype.TextValue(row.TargetType), TargetID: sqltype.TextValue(row.TargetID),
		TargetDisplay: sqltype.TextValue(row.TargetDisplay), RequestID: sqltype.TextValue(row.RequestID),
		TraceID: sqltype.TextValue(row.TraceID), SpanID: sqltype.TextValue(row.SpanID),
		IPAddress: row.IpAddress, UserAgent: row.UserAgent, Metadata: metadata,
		Changes: changes, CreatedAt: sqltype.TimeValue(row.CreatedAt),
	}, nil
}
