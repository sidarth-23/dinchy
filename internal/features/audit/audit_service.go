package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
)

type Service struct {
	store  Store
	stream StreamStore
	idg    *id.Generator
	cfg    config.AuditConfig
}

func NewService(store Store, stream StreamStore, idg *id.Generator, cfg config.AuditConfig) (*Service, error) {
	if !cfg.Enabled {
		return &Service{store: store, idg: idg, cfg: cfg}, nil
	}
	if stream == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("audit stream store is required when audit is enabled")))
	}
	return &Service{store: store, stream: stream, idg: idg, cfg: cfg}, nil
}

func (s *Service) Enabled() bool {
	return s != nil && s.cfg.Enabled
}

func (s *Service) EnsureConsumerGroup(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	if err := s.stream.CreateConsumerGroup(ctx, s.cfg.StreamName, s.cfg.ConsumerGroup); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}

func (s *Service) Record(ctx context.Context, event Event) error {
	if !s.Enabled() {
		return nil
	}
	if event.ID == "" {
		event.ID = s.idg.New()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.Metadata = sanitizeMap(event.Metadata)
	event.Changes = sanitizeMap(event.Changes)
	payload, err := json.Marshal(event)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("marshal audit event %q: %w", event.EventType, err)))
	}
	if _, err := s.stream.AddStream(ctx, s.cfg.StreamName, map[string]any{"payload": string(payload)}, s.cfg.RetentionMaxLen); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}

func (s *Service) Process(ctx context.Context) (int64, error) {
	if !s.Enabled() {
		return 0, nil
	}
	messages, err := s.stream.ReadGroup(ctx, s.cfg.StreamName, s.cfg.ConsumerGroup, s.cfg.ConsumerName, int64(s.cfg.BatchSize), 500*time.Millisecond)
	if err != nil {
		return 0, apperrors.Annotate(err)
	}
	var processed int64
	for _, message := range messages {
		payload := message.Values["payload"]
		var event Event
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return processed, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("decode audit stream message %q: %w", message.ID, err)))
		}
			params, err := insertParams(event)
			if err != nil {
				return processed, err
			}
			if err := s.store.InsertAuditLog(ctx, params); err != nil {
				return processed, apperrors.Annotate(err)
			}
		if err := s.stream.AckStream(ctx, s.cfg.StreamName, s.cfg.ConsumerGroup, message.ID); err != nil {
			return processed, apperrors.Annotate(err)
		}
		processed++
	}
	return processed, nil
}

func (s *Service) List(ctx context.Context, in ListInput) ([]Log, error) {
	if in.Limit <= 0 || in.Limit > 200 {
		in.Limit = 50
	}
	rows, err := s.store.ListAuditLogs(ctx, sqlcgen.ListAuditLogsParams{
		Column1:     in.Category,
		Category:    in.Category,
		Column3:     in.Subcategory,
		Subcategory: in.Subcategory,
		Column5:     in.EventType,
		EventType:   in.EventType,
		Column7:     in.ActorUserID,
		ActorUserID: uuid.NullUUID{UUID: mustParseUUIDMaybe(in.ActorUserID), Valid: in.ActorUserID != ""},
		Column9:     in.TargetType,
		TargetType:  sql.NullString{String: in.TargetType, Valid: in.TargetType != ""},
		Column11:    in.TargetID,
		TargetID:    sql.NullString{String: in.TargetID, Valid: in.TargetID != ""},
		Column13:    in.Outcome,
		Outcome:     in.Outcome,
		Limit:       int32(in.Limit),
		Before:      sql.NullTime{Time: in.Before.UTC(), Valid: in.BeforeValid},
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
		TargetType:          sql.NullString{String: event.TargetType, Valid: event.TargetType != ""},
		TargetID:            sql.NullString{String: event.TargetID, Valid: event.TargetID != ""},
		TargetDisplay:       sql.NullString{String: event.TargetDisplay, Valid: event.TargetDisplay != ""},
		RequestID:           sql.NullString{String: event.RequestID, Valid: event.RequestID != ""},
		TraceID:             sql.NullString{String: event.TraceID, Valid: event.TraceID != ""},
		SpanID:              sql.NullString{String: event.SpanID, Valid: event.SpanID != ""},
		IpAddress:           event.IPAddress,
		UserAgent:           event.UserAgent,
		MetadataJson:        metadataJSON,
		ChangesJson:         changesJSON,
		CreatedAt:           event.CreatedAt.UTC(),
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
		TargetType:          validString(row.TargetType.String, row.TargetType.Valid), TargetID: validString(row.TargetID.String, row.TargetID.Valid),
		TargetDisplay: validString(row.TargetDisplay.String, row.TargetDisplay.Valid), RequestID: validString(row.RequestID.String, row.RequestID.Valid),
		TraceID: validString(row.TraceID.String, row.TraceID.Valid), SpanID: validString(row.SpanID.String, row.SpanID.Valid),
		IPAddress: row.IpAddress, UserAgent: row.UserAgent, Metadata: metadata,
		Changes: changes, CreatedAt: row.CreatedAt.UTC(),
	}, nil
}

func validString(value string, valid bool) string {
	if !valid {
		return ""
	}
	return value
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

func sanitizeMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if isSensitive(key) {
			out[key] = map[string]any{"redacted": true}
			continue
		}
		out[key] = value
	}
	return out
}

func isSensitive(key string) bool {
	switch key {
	case "password", "token", "secret", "client_secret", "cookie", "session", "totp_secret", "smtp_password":
		return true
	default:
		return false
	}
}
