package events

//go:generate go run ../../cmd/codegen event -input catalog -output generated.go

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sidarth-23/dinchy/internal/platform/requestcontext"
)

// Type is the catalog identifier for an event.
type Type string

// Envelope carries the actor, target, and request context shared by every event.
type Envelope struct {
	ID                  string
	ActorUserID         string
	ActorOrganizationID string
	TargetType          string
	TargetID            string
	TargetDisplay       string
	RequestID           string
	TraceID             string
	SpanID              string
	IPAddress           string
	UserAgent           string
	CreatedAt           time.Time
}

type mappable interface {
	Map() map[string]any
}

// NoChanges is the empty change set for events that record no field-level changes.
type NoChanges struct{}

// Map returns an empty change map.
func (NoChanges) Map() map[string]any {
	return map[string]any{}
}

// Change captures the previous and current value of a single changed field.
type Change[T any] struct {
	Previous T
	Current  T
}

// TypedEvent binds an event type to its envelope and typed metadata and changes.
type TypedEvent[M, C mappable] struct {
	EventType Type
	Envelope  Envelope
	Metadata  M
	Changes   C
}

// Type returns the event's catalog type.
func (value TypedEvent[M, C]) Type() Type {
	return value.EventType
}

// EnvelopeData returns the event's envelope.
func (value TypedEvent[M, C]) EnvelopeData() Envelope {
	return value.Envelope
}

// MetadataMap returns the event metadata as a plain map.
func (value TypedEvent[M, C]) MetadataMap() map[string]any {
	return value.Metadata.Map()
}

// ChangesMap returns the event changes as a plain map.
func (value TypedEvent[M, C]) ChangesMap() map[string]any {
	return value.Changes.Map()
}

// Event is the behavior every published event provides.
type Event interface {
	Type() Type
	EnvelopeData() Envelope
	MetadataMap() map[string]any
	ChangesMap() map[string]any
}

// Definition is the catalog metadata describing a single event type.
type Definition struct {
	ID           string
	Type         Type
	Path         []string
	Category     string
	Subcategory  string
	Action       string
	Outcome      string
	Description  string
	MetadataKeys []string
	ChangeKeys   []string
}

// DefinitionFor returns the catalog definition for eventType and whether it exists.
func DefinitionFor(eventType Type) (Definition, bool) {
	definition, ok := Definitions[eventType]
	return definition, ok
}

// Target identifies the thing an event applies to.
type Target struct {
	targetType    string
	targetID      string
	targetDisplay string
}

// NewTarget constructs a target reference for an event envelope.
func NewTarget(targetType, targetID, targetDisplay string) Target {
	return Target{
		targetType:    strings.TrimSpace(targetType),
		targetID:      strings.TrimSpace(targetID),
		targetDisplay: strings.TrimSpace(targetDisplay),
	}
}

func (target Target) validate() error {
	if strings.TrimSpace(target.targetType) == "" {
		return fmt.Errorf("target type is required")
	}
	if strings.TrimSpace(target.targetID) == "" {
		return fmt.Errorf("target ID is required")
	}
	if strings.TrimSpace(target.targetDisplay) == "" {
		return fmt.Errorf("target display is required")
	}
	return nil
}

// NewEnvelope creates an event envelope and requires a populated target reference.
func NewEnvelope(ctx context.Context, actorUserID, actorOrganizationID string, target Target) (Envelope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := target.validate(); err != nil {
		return Envelope{}, err
	}
	return Envelope{
		ActorUserID:         actorUserID,
		ActorOrganizationID: actorOrganizationID,
		TargetType:          target.targetType,
		TargetID:            target.targetID,
		TargetDisplay:       target.targetDisplay,
		RequestID:           requestcontext.RequestIDFrom(ctx),
		TraceID:             requestcontext.TraceIDFrom(ctx),
		SpanID:              requestcontext.SpanIDFrom(ctx),
		IPAddress:           requestcontext.RemoteIPFrom(ctx),
		UserAgent:           requestcontext.UserAgentFrom(ctx),
	}, nil
}

// Record is the serialized envelope of a published event as stored on the stream.
type Record struct {
	ID                  string         `json:"id"`
	EventType           string         `json:"event_type"`
	Category            string         `json:"category"`
	Subcategory         string         `json:"subcategory"`
	Action              string         `json:"action"`
	Outcome             string         `json:"outcome"`
	ActorUserID         string         `json:"actor_user_id,omitempty"`
	ActorOrganizationID string         `json:"actor_organization_id,omitempty"`
	TargetType          string         `json:"target_type"`
	TargetID            string         `json:"target_id"`
	TargetDisplay       string         `json:"target_display"`
	RequestID           string         `json:"request_id,omitempty"`
	TraceID             string         `json:"trace_id,omitempty"`
	SpanID              string         `json:"span_id,omitempty"`
	IPAddress           string         `json:"ip_address,omitempty"`
	UserAgent           string         `json:"user_agent,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
	Changes             map[string]any `json:"changes,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
}
