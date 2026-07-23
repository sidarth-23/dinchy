package logging

import (
	"log/slog"
	"reflect"
)

type redactedValue struct {
	value any
}

// Redacted marks any value as sensitive for log output.
// In production it renders as a placeholder, while development mode can reveal
// the underlying value when logging is initialized with reveal enabled.
func Redacted(value any) any {
	return redactedValue{value: value}
}

func (v redactedValue) LogValue() slog.Value {
	if revealRedacted.Load() {
		if value := reflect.ValueOf(v.value); !value.IsValid() {
			return slog.AnyValue(nil)
		} else if value.Kind() == reflect.Pointer || value.Kind() == reflect.Map || value.Kind() == reflect.Slice || value.Kind() == reflect.Interface || value.Kind() == reflect.Func || value.Kind() == reflect.Chan {
			if value.IsNil() {
				return slog.AnyValue(nil)
			}
		}
		return slog.AnyValue(v.value)
	}
	return slog.StringValue("[redacted]")
}
