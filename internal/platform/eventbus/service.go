package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/cache/core"
	"github.com/sidarth-23/dinchy/internal/platform/id"
)

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

type Publisher interface {
	Publish(ctx context.Context, event events.Event) error
}

type Subscriber interface {
	Name() string
	Handle(ctx context.Context, event Event) error
}

type Service struct {
	stream      core.StreamStore
	idg         *id.Generator
	cfg         Config
	subscribers map[string]Subscriber
}

func NewService(stream core.StreamStore, idg *id.Generator, cfg Config) (*Service, error) {
	if stream == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("event stream store is required")))
	}
	if idg == nil {
		idg = id.NewGenerator()
	}
	return &Service{
		stream:      stream,
		idg:         idg,
		cfg:         cfg,
		subscribers: map[string]Subscriber{},
	}, nil
}

func (s *Service) Register(subscriber Subscriber) {
	if s == nil || subscriber == nil {
		return
	}
	s.subscribers[subscriber.Name()] = subscriber
}

func (s *Service) Subscriber(name string) (Subscriber, bool) {
	if s == nil {
		return nil, false
	}
	subscriber, ok := s.subscribers[name]
	return subscriber, ok
}

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

func (s *Service) EnsureConsumerGroups(ctx context.Context) error {
	for _, name := range s.SubscriberNames() {
		if err := s.stream.CreateConsumerGroup(ctx, s.cfg.StreamName, s.consumerGroupName(name)); err != nil {
			return apperrors.Annotate(err)
		}
	}
	return nil
}

func (s *Service) Publish(ctx context.Context, event events.Event) error {
	if s == nil {
		return nil
	}
	if event == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("event is required")))
	}
	definition, ok := events.DefinitionFor(event.Type())
	if !ok {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("event type %q is not defined in the catalog", event.Type())))
	}
	wireEvent := newWireEvent(event, definition)
	if wireEvent.ID == "" {
		wireEvent.ID = s.idg.New()
	}
	if wireEvent.CreatedAt.IsZero() {
		wireEvent.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(wireEvent)
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("marshal event %q: %w", event.Type(), err)))
	}
	if _, err := s.stream.AddStream(ctx, s.cfg.StreamName, map[string]any{"payload": string(payload)}, s.cfg.RetentionWindow); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}

func (s *Service) ProcessSubscriber(ctx context.Context, name string) (int64, error) {
	if s == nil {
		return 0, nil
	}
	subscriber, ok := s.subscribers[name]
	if !ok {
		return 0, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("subscriber %q is not registered", name)))
	}
	messages, err := s.stream.ReadGroup(ctx, s.cfg.StreamName, s.consumerGroupName(name), s.cfg.ConsumerName, int64(s.cfg.BatchSize), s.cfg.ReadBlock, s.cfg.ClaimMinIdle)
	if err != nil {
		return 0, apperrors.Annotate(err)
	}
	var processed int64
	for _, message := range messages {
		payload := message.Values["payload"]
		var event Event
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return processed, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("decode event stream message %q for subscriber %q: %w", message.ID, name, err)))
		}
		if err := subscriber.Handle(ctx, event); err != nil {
			return processed, apperrors.Annotate(err)
		}
		if err := s.stream.AckStream(ctx, s.cfg.StreamName, s.consumerGroupName(name), message.ID); err != nil {
			return processed, apperrors.Annotate(err)
		}
		processed++
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

func newWireEvent(event events.Event, definition events.Definition) Event {
	envelope := event.EnvelopeData()
	return Event{
		ID:                  envelope.ID,
		EventType:           string(event.Type()),
		Category:            definition.Category,
		Subcategory:         definition.Subcategory,
		Action:              definition.Action,
		Outcome:             definition.Outcome,
		ActorUserID:         envelope.ActorUserID,
		ActorOrganisationID: envelope.ActorOrganisationID,
		TargetType:          envelope.TargetType,
		TargetID:            envelope.TargetID,
		TargetDisplay:       envelope.TargetDisplay,
		RequestID:           envelope.RequestID,
		TraceID:             envelope.TraceID,
		SpanID:              envelope.SpanID,
		IPAddress:           envelope.IPAddress,
		UserAgent:           envelope.UserAgent,
		Metadata:            sanitizeMap(event.MetadataMap()),
		Changes:             sanitizeMap(event.ChangesMap()),
		CreatedAt:           envelope.CreatedAt,
	}
}

var _ Publisher = (*Service)(nil)
