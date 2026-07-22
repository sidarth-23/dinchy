// Package audit records and serves audit log entries for security-relevant events.
package audit

import (
	"encoding/json"
	"fmt"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

func marshalMap(kind, eventType string, value map[string]any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("marshal %s for event type %q: %w", kind, eventType, err)))
	}
	return string(raw), nil
}

func unmarshalMap(kind, eventType, raw string) (map[string]any, error) {
	out := map[string]any{}
	if raw == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("unmarshal %s for event type %q: %w", kind, eventType, err)))
	}
	return out, nil
}
