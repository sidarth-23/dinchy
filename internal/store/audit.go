package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/postgres/sqlcgen"
)

func (q *queries) InsertAuditLog(ctx context.Context, arg core.InsertAuditLogParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	actorUserID, err := nullUUID(arg.ActorUserID, arg.ActorUserIDValid)
	if err != nil {
		return err
	}
	actorOrganisationID, err := nullUUID(arg.ActorOrganisationID, arg.ActorOrganisationIDValid)
	if err != nil {
		return err
	}
	return q.q.InsertAuditLog(ctx, sqlcgen.InsertAuditLogParams{
		ID:                  id,
		Category:            arg.Category,
		Subcategory:         arg.Subcategory,
		EventType:           arg.EventType,
		Action:              arg.Action,
		Outcome:             arg.Outcome,
		ActorUserID:         actorUserID,
		ActorOrganisationID: actorOrganisationID,
		TargetType:          sql.NullString{String: arg.TargetType, Valid: arg.TargetTypeValid},
		TargetID:            sql.NullString{String: arg.TargetID, Valid: arg.TargetIDValid},
		TargetDisplay:       sql.NullString{String: arg.TargetDisplay, Valid: arg.TargetDisplayValid},
		RequestID:           sql.NullString{String: arg.RequestID, Valid: arg.RequestIDValid},
		TraceID:             sql.NullString{String: arg.TraceID, Valid: arg.TraceIDValid},
		SpanID:              sql.NullString{String: arg.SpanID, Valid: arg.SpanIDValid},
		IpAddress:           arg.IPAddress,
		UserAgent:           arg.UserAgent,
		MetadataJson:        arg.MetadataJSON,
		ChangesJson:         arg.ChangesJSON,
		CreatedAt:           arg.CreatedAt.UTC(),
	})
}

func (q *queries) ListAuditLogs(ctx context.Context, arg core.ListAuditLogsParams) ([]core.AuditLogRow, error) {
	actorUserID, err := nullUUID(arg.ActorUserID, arg.ActorUserID != "")
	if err != nil {
		return nil, err
	}
	rows, err := q.q.ListAuditLogs(ctx, sqlcgen.ListAuditLogsParams{
		Column1:     arg.Category,
		Category:    arg.Category,
		Column3:     arg.Subcategory,
		Subcategory: arg.Subcategory,
		Column5:     arg.EventType,
		EventType:   arg.EventType,
		Column7:     arg.ActorUserID,
		ActorUserID: actorUserID,
		Column9:     arg.TargetType,
		TargetType:  sql.NullString{String: arg.TargetType, Valid: arg.TargetType != ""},
		Column11:    arg.TargetID,
		TargetID:    sql.NullString{String: arg.TargetID, Valid: arg.TargetID != ""},
		Column13:    arg.Outcome,
		Outcome:     arg.Outcome,
		Limit:       int32(arg.Limit),
		Before:      sql.NullTime{Time: arg.Before.UTC(), Valid: arg.BeforeValid},
	})
	if err != nil {
		return nil, err
	}
	out := make([]core.AuditLogRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, auditLogRow(row))
	}
	return out, nil
}

func nullUUID(value string, valid bool) (uuid.NullUUID, error) {
	if !valid {
		return uuid.NullUUID{}, nil
	}
	parsed, err := parseUUID(value)
	if err != nil {
		return uuid.NullUUID{}, err
	}
	return uuid.NullUUID{UUID: parsed, Valid: true}, nil
}

func auditLogRow(row sqlcgen.AppAuditLog) core.AuditLogRow {
	return core.AuditLogRow{
		ID: row.ID.String(), Category: row.Category, Subcategory: row.Subcategory, EventType: row.EventType,
		Action: row.Action, Outcome: row.Outcome, ActorUserID: row.ActorUserID.UUID.String(),
		ActorUserIDValid: row.ActorUserID.Valid, ActorOrganisationID: row.ActorOrganisationID.UUID.String(),
		ActorOrganisationIDValid: row.ActorOrganisationID.Valid, TargetType: row.TargetType.String,
		TargetTypeValid: row.TargetType.Valid, TargetID: row.TargetID.String, TargetIDValid: row.TargetID.Valid,
		TargetDisplay: row.TargetDisplay.String, TargetDisplayValid: row.TargetDisplay.Valid,
		RequestID: row.RequestID.String, RequestIDValid: row.RequestID.Valid, TraceID: row.TraceID.String,
		TraceIDValid: row.TraceID.Valid, SpanID: row.SpanID.String, SpanIDValid: row.SpanID.Valid,
		IPAddress: row.IpAddress, UserAgent: row.UserAgent, MetadataJSON: row.MetadataJson,
		ChangesJSON: row.ChangesJson, CreatedAt: row.CreatedAt.UTC(),
	}
}

var _ = time.Time{}
