package i18n

import (
	"fmt"
	"reflect"
	"strings"
)

// Validate checks that every string field in msgs is non-empty.
// Returns an error listing all fields with their msg tags that are missing translations.
func Validate(msgs Messages) error {
	v := reflect.ValueOf(msgs)
	t := v.Type()

	var missing []string
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Type.Kind() != reflect.String {
			continue
		}
		if v.Field(i).String() == "" {
			tag := field.Tag.Get("msg")
			missing = append(missing, fmt.Sprintf("%s (msg:%q)", field.Name, tag))
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("i18n: missing translations: %s", strings.Join(missing, ", "))
	}
	return nil
}
