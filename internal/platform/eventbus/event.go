package eventbus

import "time"

type Event struct {
	ID                  string         `json:"id"`
	EventType           string         `json:"event_type"`
	Category            string         `json:"category"`
	Subcategory         string         `json:"subcategory"`
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
