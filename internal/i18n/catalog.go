// Package i18n provides a translation catalog for resolving error codes to
// human-readable messages in a requested locale.
package i18n

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
	"text/template"

	"golang.org/x/text/language"
)

// Catalog maps error codes to localized messages across multiple locales.
type Catalog struct {
	mu       sync.RWMutex
	locales  map[language.Tag]map[string]string
	fallback language.Tag
	matcher  language.Matcher
}

// New creates a Catalog with the given fallback locale.
func New(fallback language.Tag) *Catalog {
	return &Catalog{
		locales:  make(map[language.Tag]map[string]string),
		fallback: fallback,
		matcher:  language.NewMatcher([]language.Tag{fallback}),
	}
}

// Register adds a locale's Messages to the catalog and rebuilds the matcher.
// Panics if any field in messages is empty (missing translation).
func (c *Catalog) Register(tag language.Tag, messages Messages) {
	if err := Validate(messages); err != nil {
		panic(fmt.Sprintf("i18n: registering %s: %v", tag, err))
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.locales[tag] = messageMap(messages)

	tags := make([]language.Tag, 0, len(c.locales))
	for t := range c.locales {
		tags = append(tags, t)
	}
	c.matcher = language.NewMatcher(tags)
}

// Match parses an Accept-Language header value and returns the best supported tag.
func (c *Catalog) Match(acceptLanguage string) language.Tag {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if acceptLanguage == "" {
		return c.fallback
	}
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil || len(tags) == 0 {
		return c.fallback
	}
	tag, _, _ := c.matcher.Match(tags...)
	return tag
}

// Resolve returns the localized message for code in the requested locale.
// It falls back to the catalog fallback locale, then to the code itself.
func (c *Catalog) Resolve(tag language.Tag, code string, meta map[string]any) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if msg, ok := c.lookup(tag, code); ok {
		return render(msg, meta)
	}

	if tag != c.fallback {
		if msg, ok := c.lookup(c.fallback, code); ok {
			return render(msg, meta)
		}
	}

	return code
}

func (c *Catalog) lookup(tag language.Tag, code string) (string, bool) {
	msgs, ok := c.locales[tag]
	if !ok {
		return "", false
	}
	msg, ok := msgs[code]
	return msg, ok
}

func messageMap(messages Messages) map[string]string {
	out := make(map[string]string)
	v := reflect.ValueOf(messages)
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("msg")
		if tag == "" {
			continue
		}
		out[tag] = v.Field(i).String()
	}
	return out
}

func render(tpl string, meta map[string]any) string {
	if tpl == "" {
		return ""
	}
	if len(meta) == 0 {
		return tpl
	}
	t, err := template.New("message").Option("missingkey=zero").Parse(tpl)
	if err != nil {
		return tpl
	}
	var b bytes.Buffer
	if err := t.Execute(&b, meta); err != nil {
		return tpl
	}
	return b.String()
}
