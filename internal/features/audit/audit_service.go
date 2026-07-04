package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
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
	rows, err := s.store.ListAuditLogs(ctx, ListAuditLogsParams{
		Category: in.Category, Subcategory: in.Subcategory, EventType: in.EventType,
		ActorUserID: in.ActorUserID, TargetType: in.TargetType, TargetID: in.TargetID,
		Outcome: in.Outcome, Before: in.Before, BeforeValid: in.BeforeValid, Limit: in.Limit,
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

func insertParams(event Event) (InsertAuditLogParams, error) {
	metadataJSON, err := marshalMap("audit metadata", event.EventType, event.Metadata)
	if err != nil {
		return InsertAuditLogParams{}, err
	}
	changesJSON, err := marshalMap("audit changes", event.EventType, event.Changes)
	if err != nil {
		return InsertAuditLogParams{}, err
	}
	return InsertAuditLogParams{
		ID: event.ID, Category: event.Category, Subcategory: event.Subcategory, EventType: event.EventType,
		Action: event.Action, Outcome: event.Outcome, ActorUserID: event.ActorUserID,
		ActorUserIDValid: event.ActorUserID != "", ActorOrganisationID: event.ActorOrganisationID,
		ActorOrganisationIDValid: event.ActorOrganisationID != "", TargetType: event.TargetType,
		TargetTypeValid: event.TargetType != "", TargetID: event.TargetID, TargetIDValid: event.TargetID != "",
		TargetDisplay: event.TargetDisplay, TargetDisplayValid: event.TargetDisplay != "",
		RequestID: event.RequestID, RequestIDValid: event.RequestID != "", TraceID: event.TraceID,
		TraceIDValid: event.TraceID != "", SpanID: event.SpanID, SpanIDValid: event.SpanID != "",
		IPAddress: event.IPAddress, UserAgent: event.UserAgent, MetadataJSON: metadataJSON,
		ChangesJSON: changesJSON, CreatedAt: event.CreatedAt,
	}, nil
}

func logFromRow(row AuditLogRow) (Log, error) {
	metadata, err := unmarshalMap("audit metadata", row.EventType, row.MetadataJSON)
	if err != nil {
		return Log{}, err
	}
	changes, err := unmarshalMap("audit changes", row.EventType, row.ChangesJSON)
	if err != nil {
		return Log{}, err
	}
	return Log{
		ID: row.ID, Category: row.Category, Subcategory: row.Subcategory, EventType: row.EventType,
		Action: row.Action, Outcome: row.Outcome, ActorUserID: validString(row.ActorUserID, row.ActorUserIDValid),
		ActorOrganisationID: validString(row.ActorOrganisationID, row.ActorOrganisationIDValid),
		TargetType:          validString(row.TargetType, row.TargetTypeValid), TargetID: validString(row.TargetID, row.TargetIDValid),
		TargetDisplay: validString(row.TargetDisplay, row.TargetDisplayValid), RequestID: validString(row.RequestID, row.RequestIDValid),
		TraceID: validString(row.TraceID, row.TraceIDValid), SpanID: validString(row.SpanID, row.SpanIDValid),
		IPAddress: row.IPAddress, UserAgent: row.UserAgent, Metadata: metadata,
		Changes: changes, CreatedAt: row.CreatedAt,
	}, nil
}

func validString(value string, valid bool) string {
	if !valid {
		return ""
	}
	return value
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
