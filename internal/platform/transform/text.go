// Package transform provides small pure string normalization helpers.
package transform

import "strings"

// Spec constants name the modifier pipelines callers reuse so intent is declared
// once instead of repeating literals. They are validated at package init, so
// ApplyTo never panics and Apply never returns ok=false when given one of them.
const (
	SpecEmail = "trim,lower"
	SpecTrim  = "trim"
)

func init() {
	for _, spec := range []string{SpecEmail, SpecTrim} {
		if _, ok := Apply(spec, ""); !ok {
			panic("transform: spec constant references an unknown modifier: " + spec)
		}
	}
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
// silently ignoring it. Use it when spec comes from external input (such as a
// struct tag); use ApplyTo when spec is a statically-known constant.
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

// ApplyTo runs spec over the pointed-to string in place. It is the reusable
// primitive for request resolvers across features: pass one of the Spec
// constants and the field to normalize, e.g. ApplyTo(SpecEmail, &body.Email).
// It panics if spec names an unknown modifier, so it must only be used with
// statically-known specs (the Spec constants are validated at package init);
// use Apply for specs that originate from external input.
func ApplyTo(spec string, value *string) {
	result, ok := Apply(spec, *value)
	if !ok {
		panic("transform: unknown modifier in spec: " + spec)
	}
	*value = result
}
