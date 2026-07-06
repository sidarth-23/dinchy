package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/eventbus"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

type Service struct {
	store Store
	idg   *id.Generator
}

func NewService(store Store) (*Service, error) {
	if store == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("audit store is required")))
	}
	return &Service{store: store, idg: id.NewGenerator()}, nil
}

func (s *Service) Name() string {
	return "audit"
}

func (s *Service) Handle(ctx context.Context, event eventbus.Event) error {
	if event.ID == "" {
		event.ID = s.idg.New()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = nowUTC()
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

var _ eventbus.Subscriber = (*Service)(nil)

func (s *Service) List(ctx context.Context, in ListInput) ([]Log, error) {
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}
	rows, err := s.store.ListAuditLogs(ctx, sqlcgen.ListAuditLogsParams{
		CategoryFilter:    in.Category,
		Category:          in.Category,
		SubcategoryFilter: in.Subcategory,
		Subcategory:       in.Subcategory,
		EventTypeFilter:   in.EventType,
		EventType:         in.EventType,
		ActorUserIDFilter: in.ActorUserID,
		ActorUserID:       uuid.NullUUID{UUID: mustParseUUIDMaybe(in.ActorUserID), Valid: in.ActorUserID != ""},
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
	out := make([]Log, 0, len(rows))
	for _, row := range rows {
		log, err := logFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, nil
}

func insertParams(event Event) (sqlcgen.InsertAuditLogParams, error) {
	metadataJSON, err := marshalMap("audit metadata", event.EventType, event.Metadata)
	if err != nil {
		return sqlcgen.InsertAuditLogParams{}, err
	}
	changesJSON, err := marshalMap("audit changes", event.EventType, event.Changes)
	if err != nil {
		return sqlcgen.InsertAuditLogParams{}, err
	}
	return sqlcgen.InsertAuditLogParams{
		ID:                  mustParseUUID(event.ID),
		Category:            event.Category,
		Subcategory:         event.Subcategory,
		EventType:           event.EventType,
		Action:              event.Action,
		Outcome:             event.Outcome,
		ActorUserID:         uuid.NullUUID{UUID: mustParseUUIDMaybe(event.ActorUserID), Valid: event.ActorUserID != ""},
		ActorOrganisationID: uuid.NullUUID{UUID: mustParseUUIDMaybe(event.ActorOrganisationID), Valid: event.ActorOrganisationID != ""},
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

func logFromRow(row sqlcgen.AppAuditLog) (Log, error) {
	metadata, err := unmarshalMap("audit metadata", row.EventType, row.MetadataJson)
	if err != nil {
		return Log{}, err
	}
	changes, err := unmarshalMap("audit changes", row.EventType, row.ChangesJson)
	if err != nil {
		return Log{}, err
	}
	return Log{
		ID: row.ID.String(), Category: row.Category, Subcategory: row.Subcategory, EventType: row.EventType,
		Action: row.Action, Outcome: row.Outcome, ActorUserID: validUUID(row.ActorUserID),
		ActorOrganisationID: validUUID(row.ActorOrganisationID),
		TargetType:          sqltype.TextValue(row.TargetType), TargetID: sqltype.TextValue(row.TargetID),
		TargetDisplay: sqltype.TextValue(row.TargetDisplay), RequestID: sqltype.TextValue(row.RequestID),
		TraceID: sqltype.TextValue(row.TraceID), SpanID: sqltype.TextValue(row.SpanID),
		IPAddress: row.IpAddress, UserAgent: row.UserAgent, Metadata: metadata,
		Changes: changes, CreatedAt: sqltype.TimeValue(row.CreatedAt),
	}, nil
}

func validUUID(value uuid.NullUUID) string {
	if !value.Valid {
		return ""
	}
	return value.UUID.String()
}

func mustParseUUID(value string) uuid.UUID {
	parsed, err := id.Parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func mustParseUUIDMaybe(value string) uuid.UUID {
	if value == "" {
		return uuid.Nil
	}
	return mustParseUUID(value)
}

func marshalMap(kind, eventType string, value map[string]any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("marshal %s for event type %q: %w", kind, eventType, err)))
	}
	return string(raw), nil
}

func unmarshalMap(kind, eventType, raw string) (map[string]any, error) {
	out := map[string]any{}
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("unmarshal %s for event type %q: %w", kind, eventType, err)))
	}
	return out, nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
