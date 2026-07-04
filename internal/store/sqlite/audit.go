package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

func (q *queries) InsertAuditLog(ctx context.Context, arg core.InsertAuditLogParams) error {
	return q.q.InsertAuditLog(ctx, sqlcgen.InsertAuditLogParams{
		ID:                  arg.ID,
		Category:            arg.Category,
		Subcategory:         arg.Subcategory,
		EventType:           arg.EventType,
		Action:              arg.Action,
		Outcome:             arg.Outcome,
		ActorUserID:         sql.NullString{String: arg.ActorUserID, Valid: arg.ActorUserIDValid},
		ActorOrganisationID: sql.NullString{String: arg.ActorOrganisationID, Valid: arg.ActorOrganisationIDValid},
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
		CreatedAt:           formatTime(arg.CreatedAt),
	})
}

func (q *queries) ListAuditLogs(ctx context.Context, arg core.ListAuditLogsParams) ([]core.AuditLogRow, error) {
	before := ""
	if arg.BeforeValid {
		before = formatTime(arg.Before)
	}
	rows, err := q.q.ListAuditLogs(ctx, sqlcgen.ListAuditLogsParams{
		Column1:     arg.Category,
		Category:    arg.Category,
		Column3:     arg.Subcategory,
		Subcategory: arg.Subcategory,
		Column5:     arg.EventType,
		EventType:   arg.EventType,
		Column7:     arg.ActorUserID,
		ActorUserID: sql.NullString{String: arg.ActorUserID, Valid: arg.ActorUserID != ""},
		Column9:     arg.TargetType,
		TargetType:  sql.NullString{String: arg.TargetType, Valid: arg.TargetType != ""},
		Column11:    arg.TargetID,
		TargetID:    sql.NullString{String: arg.TargetID, Valid: arg.TargetID != ""},
		Column13:    arg.Outcome,
		Outcome:     arg.Outcome,
		Column15:    before,
		CreatedAt:   before,
		Limit:       arg.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]core.AuditLogRow, 0, len(rows))
	for _, row := range rows {
		createdAt, err := parseTime(row.CreatedAt)
		if err != nil {
			return nil, wrapParseErr("app_audit_logs.created_at", err)
		}
		out = append(out, auditLogRow(row, createdAt))
	}
	return out, nil
}

func auditLogRow(row sqlcgen.AppAuditLog, createdAt time.Time) core.AuditLogRow {
	return core.AuditLogRow{
		ID: row.ID, Category: row.Category, Subcategory: row.Subcategory, EventType: row.EventType,
		Action: row.Action, Outcome: row.Outcome, ActorUserID: row.ActorUserID.String,
		ActorUserIDValid: row.ActorUserID.Valid, ActorOrganisationID: row.ActorOrganisationID.String,
		ActorOrganisationIDValid: row.ActorOrganisationID.Valid, TargetType: row.TargetType.String,
		TargetTypeValid: row.TargetType.Valid, TargetID: row.TargetID.String, TargetIDValid: row.TargetID.Valid,
		TargetDisplay: row.TargetDisplay.String, TargetDisplayValid: row.TargetDisplay.Valid,
		RequestID: row.RequestID.String, RequestIDValid: row.RequestID.Valid, TraceID: row.TraceID.String,
		TraceIDValid: row.TraceID.Valid, SpanID: row.SpanID.String, SpanIDValid: row.SpanID.Valid,
		IPAddress: row.IpAddress, UserAgent: row.UserAgent, MetadataJSON: row.MetadataJson,
		ChangesJSON: row.ChangesJson, CreatedAt: createdAt,
	}
}
