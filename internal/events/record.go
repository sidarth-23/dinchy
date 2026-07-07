package events

import "time"

// Record is the serialized envelope of a published event as stored on the stream.
type Record struct {
	ID                  string         `json:"id"`
	EventType           string         `json:"event_type"`
	Category            string         `json:"category"`
	Subcategory         string         `json:"subcategory"`
	Action              string         `json:"action"`
	Outcome             string         `json:"outcome"`
	ActorUserID         string         `json:"actor_user_id,omitempty"`
	ActorOrganisationID string         `json:"actor_organization_id,omitempty"`
	TargetType          string         `json:"target_type,omitempty"`
	TargetID            string         `json:"target_id,omitempty"`
	TargetDisplay       string         `json:"target_display,omitempty"`
	RequestID           string         `json:"request_id,omitempty"`
	TraceID             string         `json:"trace_id,omitempty"`
	SpanID              string         `json:"span_id,omitempty"`
	IPAddress           string         `json:"ip_address,omitempty"`
	UserAgent           string         `json:"user_agent,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	Changes             map[string]any `json:"changes,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
}
