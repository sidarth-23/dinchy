package audit

import (
	"context"
	"time"

	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

// Store persists and retrieves audit log entries.
type Store interface {
	InsertAuditLog(ctx context.Context, arg sqlcgen.InsertAuditLogParams) error
	ListAuditLogs(ctx context.Context, arg sqlcgen.ListAuditLogsParams) ([]sqlcgen.AppAuditLog, error)
}

// ListInput holds the parsed filters used to query audit logs.
type ListInput struct {
	Category    string
	Subcategory string
	EventType   string
	ActorUserID string
	TargetType  string
	TargetID    string
	Outcome     string
	Before      time.Time
	BeforeValid bool
	Limit       int64
}

// ListIn is the query-parameter request for listing audit logs.
type ListIn struct {
	Category    string    `query:"category" doc:"Filter by event category"`
	Subcategory string    `query:"subcategory" doc:"Filter by event subcategory"`
	EventType   string    `query:"event_type" doc:"Filter by event type"`
	ActorUserID string    `query:"actor_user_id" doc:"Filter by the acting user's ID"`
	TargetType  string    `query:"target_type" doc:"Filter by target type"`
	TargetID    string    `query:"target_id" doc:"Filter by target ID"`
	Outcome     string    `query:"outcome" doc:"Filter by outcome"`
	Before      time.Time `query:"before" example:"2026-01-02T15:04:05Z" doc:"Return logs created strictly before this RFC3339 timestamp"`
	Limit       int64     `query:"limit" minimum:"1" maximum:"200" default:"50" doc:"Maximum number of logs to return"`
}

// LogOut is the serialized representation of a single audit log entry.
type LogOut struct {
	ID                  string         `json:"id" doc:"Audit log entry identifier"`
	Category            string         `json:"category" doc:"Event category"`
	Subcategory         string         `json:"subcategory" doc:"Event subcategory"`
	EventType           string         `json:"event_type" doc:"Event type"`
	Action              string         `json:"action" doc:"Action performed"`
	Outcome             string         `json:"outcome" doc:"Outcome of the action"`
	ActorUserID         string         `json:"actor_user_id,omitempty" doc:"ID of the acting user"`
	ActorOrganisationID string         `json:"actor_organization_id,omitempty" doc:"ID of the acting organization"`
	TargetType          string         `json:"target_type,omitempty" doc:"Type of the affected target"`
	TargetID            string         `json:"target_id,omitempty" doc:"ID of the affected target"`
	TargetDisplay       string         `json:"target_display,omitempty" doc:"Human-readable target label"`
	RequestID           string         `json:"request_id,omitempty" doc:"Originating request ID"`
	TraceID             string         `json:"trace_id,omitempty" doc:"Distributed trace ID"`
	SpanID              string         `json:"span_id,omitempty" doc:"Distributed span ID"`
	IPAddress           string         `json:"ip_address" doc:"Client IP address"`
	UserAgent           string         `json:"user_agent" doc:"Client user agent"`
	Metadata            map[string]any `json:"metadata" doc:"Additional structured event metadata"`
	Changes             map[string]any `json:"changes" doc:"Field-level changes captured for the event"`
	CreatedAt           time.Time      `json:"created_at" doc:"Timestamp the event was recorded"`
}

// ListOut is the response body wrapping the returned audit logs.
type ListOut struct {
	Body struct {
		Logs []LogOut `json:"logs"`
	}
}
