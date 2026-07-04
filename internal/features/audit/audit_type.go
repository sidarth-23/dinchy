package audit

import (
	"context"
	"time"

	cachecore "github.com/sidarth-23/dinchy/internal/cache/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
)

const (
	CategorySecurity = "security"

	SubcategoryAuth        = "auth"
	SubcategoryTwoFactor   = "two_factor"
	SubcategorySSOSettings = "sso_settings"

	OutcomeSucceeded = "succeeded"
	OutcomeFailed    = "failed"
)

type StreamStore interface {
	CreateConsumerGroup(ctx context.Context, stream, group string) error
	AddStream(ctx context.Context, stream string, values map[string]any, maxLen int64) (string, error)
	ReadGroup(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]StreamMessage, error)
	AckStream(ctx context.Context, stream, group string, ids ...string) error
}

type StreamMessage = cachecore.StreamMessage

type Store interface {
	InsertAuditLog(ctx context.Context, arg sqlcgen.InsertAuditLogParams) error
	ListAuditLogs(ctx context.Context, arg sqlcgen.ListAuditLogsParams) ([]sqlcgen.AppAuditLog, error)
}

type InsertAuditLogParams struct {
	ID                       string
	Category                 string
	Subcategory              string
	EventType                string
	Action                   string
	Outcome                  string
	ActorUserID              string
	ActorUserIDValid         bool
	ActorOrganisationID      string
	ActorOrganisationIDValid bool
	TargetType               string
	TargetTypeValid          bool
	TargetID                 string
	TargetIDValid            bool
	TargetDisplay            string
	TargetDisplayValid       bool
	RequestID                string
	RequestIDValid           bool
	TraceID                  string
	TraceIDValid             bool
	SpanID                   string
	SpanIDValid              bool
	IPAddress                string
	UserAgent                string
	MetadataJSON             string
	ChangesJSON              string
	CreatedAt                time.Time
}

type AuditLogRow struct {
	ID                       string
	Category                 string
	Subcategory              string
	EventType                string
	Action                   string
	Outcome                  string
	ActorUserID              string
	ActorUserIDValid         bool
	ActorOrganisationID      string
	ActorOrganisationIDValid bool
	TargetType               string
	TargetTypeValid          bool
	TargetID                 string
	TargetIDValid            bool
	TargetDisplay            string
	TargetDisplayValid       bool
	RequestID                string
	RequestIDValid           bool
	TraceID                  string
	TraceIDValid             bool
	SpanID                   string
	SpanIDValid              bool
	IPAddress                string
	UserAgent                string
	MetadataJSON             string
	ChangesJSON              string
	CreatedAt                time.Time
}

type ListAuditLogsParams struct {
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

type Event struct {
	ID                  string         `json:"id"`
	Category            string         `json:"category"`
	Subcategory         string         `json:"subcategory"`
	EventType           string         `json:"event_type"`
	Action              string         `json:"action"`
	Outcome             string         `json:"outcome"`
	ActorUserID         string         `json:"actor_user_id,omitempty"`
	ActorOrganisationID string         `json:"actor_organisation_id,omitempty"`
	TargetType          string         `json:"target_type,omitempty"`
	TargetID            string         `json:"target_id,omitempty"`
	TargetDisplay       string         `json:"target_display,omitempty"`
	RequestID           string         `json:"request_id,omitempty"`
	TraceID             string         `json:"trace_id,omitempty"`
	SpanID              string         `json:"span_id,omitempty"`
	IPAddress           string         `json:"ip_address"`
	UserAgent           string         `json:"user_agent"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	Changes             map[string]any `json:"changes,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
}

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

type Log = Event

type ListIn struct {
	Category    string `query:"category"`
	Subcategory string `query:"subcategory"`
	EventType   string `query:"event_type"`
	ActorUserID string `query:"actor_user_id"`
	TargetType  string `query:"target_type"`
	TargetID    string `query:"target_id"`
	Outcome     string `query:"outcome"`
	Before      string `query:"before"`
	Limit       int64  `query:"limit" minimum:"1" maximum:"200"`
}

type LogOut struct {
	ID                  string         `json:"id"`
	Category            string         `json:"category"`
	Subcategory         string         `json:"subcategory"`
	EventType           string         `json:"event_type"`
	Action              string         `json:"action"`
	Outcome             string         `json:"outcome"`
	ActorUserID         string         `json:"actor_user_id,omitempty"`
	ActorOrganisationID string         `json:"actor_organisation_id,omitempty"`
	TargetType          string         `json:"target_type,omitempty"`
	TargetID            string         `json:"target_id,omitempty"`
	TargetDisplay       string         `json:"target_display,omitempty"`
	RequestID           string         `json:"request_id,omitempty"`
	TraceID             string         `json:"trace_id,omitempty"`
	SpanID              string         `json:"span_id,omitempty"`
	IPAddress           string         `json:"ip_address"`
	UserAgent           string         `json:"user_agent"`
	Metadata            map[string]any `json:"metadata"`
	Changes             map[string]any `json:"changes"`
	CreatedAt           time.Time      `json:"created_at"`
}

type ListOut struct {
	Body struct {
		Logs []LogOut `json:"logs"`
	}
}
