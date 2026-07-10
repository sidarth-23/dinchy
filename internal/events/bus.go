// Package events provides a Redis-stream-backed event bus that publishes domain
// events and dispatches them to registered subscribers via consumer groups.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
)

// Config holds the Redis stream and consumer-group tuning for the event bus.
type Config struct {
	StreamName          string
	ConsumerGroupPrefix string
	ConsumerName        string
	BatchSize           int
	RetentionWindow     time.Duration
	ClaimMinIdle        time.Duration
	ReadBlock           time.Duration
	WorkerInterval      time.Duration
}

// Publisher publishes an event to the bus.
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

// Subscriber consumes event records dispatched to it by name.
type Subscriber interface {
	features.Feature
	// Handle processes one event record; a returned error leaves it unacknowledged.
	Handle(ctx context.Context, event Record) error
}

// Service is a Redis-backed event bus that publishes events and dispatches them to subscribers.
type Service struct {
	client      *goredis.Client
	idg         *id.Generator
	cfg         Config
	subscribers map[string]Subscriber
}

// NewService constructs a Service; the Redis client is required and a default ID generator is used when idg is nil.
func NewService(client *goredis.Client, idg *id.Generator, cfg Config) (*Service, error) {
	if client == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("redis client is required for the event bus")))
	}
	if idg == nil {
		idg = id.NewGenerator()
	}
	return &Service{client: client, idg: idg, cfg: cfg, subscribers: map[string]Subscriber{}}, nil
}

// Register adds a subscriber, keyed by its name.
func (s *Service) Register(subscriber Subscriber) {
	if s == nil || subscriber == nil {
		return
	}
	s.subscribers[subscriber.Name()] = subscriber
}

// Subscriber returns the registered subscriber for name and whether it exists.
func (s *Service) Subscriber(name string) (Subscriber, bool) {
	if s == nil {
		return nil, false
	}
	subscriber, ok := s.subscribers[name]
	return subscriber, ok
}

// SubscriberNames returns the registered subscriber names in sorted order.
func (s *Service) SubscriberNames() []string {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.subscribers))
	for name := range s.subscribers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EnsureConsumerGroups creates the stream consumer group for each subscriber, ignoring already-existing groups.
//
//dinchy:allow-logreturn consumer-group setup returns annotated errors without owning the logging boundary
func (s *Service) EnsureConsumerGroups(ctx context.Context) error {
	for _, name := range s.SubscriberNames() {
		if err := s.client.XGroupCreateMkStream(ctx, s.cfg.StreamName, s.consumerGroupName(name), "0").Err(); err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
			return apperrors.Annotate(err)
		}
	}
	return nil
}

// Publish records the event onto the stream, assigning an ID and timestamp when absent.
func (s *Service) Publish(ctx context.Context, event Event) error {
	if s == nil {
		return nil
	}
	if event == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("event is required")))
	}
	definition, ok := DefinitionFor(event.Type())
	if !ok {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("event type %q is not defined in the catalog", event.Type())))
	}
	record := newRecord(event, definition)
	if record.ID == "" {
		record.ID = s.idg.New()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("marshal event %q: %w", event.Type(), err)))
	}
	args := &goredis.XAddArgs{Stream: s.cfg.StreamName, Values: map[string]any{"payload": string(payload)}}
	if s.cfg.RetentionWindow > 0 {
		args.MinID = fmt.Sprintf("%d-0", time.Now().UTC().Add(-s.cfg.RetentionWindow).UnixMilli())
		args.Approx = true
	}
	if err := s.client.XAdd(ctx, args).Err(); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}

// ProcessSubscriber reads and handles a batch of pending messages for the named subscriber, acking each, and returns the number processed.
func (s *Service) ProcessSubscriber(ctx context.Context, name string) (int64, error) {
	if s == nil {
		return 0, nil
	}
	subscriber, ok := s.subscribers[name]
	if !ok {
		return 0, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("subscriber %q is not registered", name)))
	}
	streams, err := s.client.XReadGroup(ctx, &goredis.XReadGroupArgs{Group: s.consumerGroupName(name), Consumer: s.cfg.ConsumerName, Streams: []string{s.cfg.StreamName, ">"}, Count: int64(s.cfg.BatchSize), Block: s.cfg.ReadBlock, Claim: s.cfg.ClaimMinIdle}).Result()
	if err != nil {
		if err == goredis.Nil {
			return 0, nil
		}
		return 0, apperrors.Annotate(err)
	}
	var processed int64
	for _, stream := range streams {
		for _, message := range stream.Messages {
			var record Record
			payload := fmt.Sprint(message.Values["payload"])
			if err := json.Unmarshal([]byte(payload), &record); err != nil {
				return processed, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("decode event stream message %q for subscriber %q: %w", message.ID, name, err)))
			}
			if err := subscriber.Handle(ctx, record); err != nil {
				return processed, apperrors.Annotate(err)
			}
			if err := s.client.XAck(ctx, s.cfg.StreamName, s.consumerGroupName(name), message.ID).Err(); err != nil {
				return processed, apperrors.Annotate(err)
			}
			processed++
		}
	}
	return processed, nil
}

func (s *Service) consumerGroupName(subscriberName string) string {
	return s.cfg.ConsumerGroupPrefix + ":" + subscriberName
}

func sanitizeMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		switch key {
		case "password", "token", "secret", "client_secret", "cookie", "session", "totp_secret", "smtp_password":
			out[key] = map[string]any{"redacted": true}
		default:
			out[key] = value
		}
	}
	return out
}

func newRecord(event Event, definition Definition) Record {
	envelope := event.EnvelopeData()
	return Record{ID: envelope.ID, EventType: string(event.Type()), Category: definition.Category, Subcategory: definition.Subcategory, Action: definition.Action, Outcome: definition.Outcome, ActorUserID: envelope.ActorUserID, ActorOrganisationID: envelope.ActorOrganisationID, TargetType: envelope.TargetType, TargetID: envelope.TargetID, TargetDisplay: envelope.TargetDisplay, RequestID: envelope.RequestID, TraceID: envelope.TraceID, SpanID: envelope.SpanID, IPAddress: envelope.IPAddress, UserAgent: envelope.UserAgent, Metadata: sanitizeMap(event.MetadataMap()), Changes: sanitizeMap(event.ChangesMap()), CreatedAt: envelope.CreatedAt}
}

var _ Publisher = (*Service)(nil)
