// Package transform provides small pure string normalization helpers.
package transform

import "strings"

func Email(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func Trim(value string) string {
	return strings.TrimSpace(value)
}

// stringMods maps a modifier name to its string transformation. It backs the
// declarative `mod:"..."` config tag so normalization is declared per field
// rather than special-cased by env key.
var stringMods = map[string]func(string) string{
	"trim":  strings.TrimSpace,
	"lower": strings.ToLower,
	"upper": strings.ToUpper,
}

// Apply runs the comma-separated modifier list (for example "trim,lower") over
// value from left to right. It returns ok=false when spec names a modifier that
// is not registered so the caller can surface a configuration error instead of
// silently ignoring it.
func Apply(spec, value string) (string, bool) {
	for name := range strings.SplitSeq(spec, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		mod, ok := stringMods[name]
		if !ok {
			return value, false
		}
		value = mod(value)
	}
	return value, true
}
