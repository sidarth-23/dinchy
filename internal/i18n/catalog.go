// Package i18n provides a simple translation catalog for resolving error codes
// to human-readable messages in a requested locale.
package i18n

import (
	"fmt"
	"sync"

	"golang.org/x/text/language"
)

// Catalog maps error codes to localized messages across multiple locales.
type Catalog struct {
	mu       sync.RWMutex
	locales  map[language.Tag]Messages
	fallback language.Tag
	matcher  language.Matcher
}

// New creates a Catalog with the given fallback locale.
func New(fallback language.Tag) *Catalog {
	return &Catalog{
		locales:  make(map[language.Tag]Messages),
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

	c.locales[tag] = messages

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

// Resolve returns the message selected by fn in the requested locale, falling back to
// the catalog's fallback locale, then to the msg tag string if no translation exists.
func (c *Catalog) Resolve(tag language.Tag, fn MsgFunc) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if msgs, ok := c.locales[tag]; ok {
		if msg := fn(msgs); msg != "" {
			return msg
		}
	}

	if tag != c.fallback {
		if msgs, ok := c.locales[c.fallback]; ok {
			if msg := fn(msgs); msg != "" {
				return msg
			}
		}
	}

	return MsgCode(fn)
}
