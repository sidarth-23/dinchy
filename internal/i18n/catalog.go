// Package i18n provides a translation catalog for resolving typed error
// messages to human-readable strings in a requested locale.
package i18n

import (
	"bytes"
	"sort"
	"text/template"

	"golang.org/x/text/language"
)

// Catalog maps error messages to localized templates across supported locales.
type Catalog struct {
	locales map[language.Tag]map[string]string
	tags    []language.Tag
	matcher language.Matcher
}

// New creates a Catalog from a locale map.
func New(locales map[language.Tag]map[string]string) *Catalog {
	cloned := cloneLocales(locales)
	tags := sortedTags(cloned)
	return &Catalog{
		locales: cloned,
		tags:    tags,
		matcher: language.NewMatcher(tags),
	}
}

// Match parses an Accept-Language header and returns the best supported tag.
func (c *Catalog) Match(acceptLanguage string) language.Tag {
	if len(c.tags) == 0 {
		return language.Und
	}
	if acceptLanguage == "" {
		return c.tags[0]
	}
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err != nil || len(tags) == 0 {
		return c.tags[0]
	}
	tag, _, _ := c.matcher.Match(tags...)
	return tag
}

// Resolve returns the localized message for msg in the requested locale.
// It performs an exact lookup only. Missing locales or codes return empty.
func (c *Catalog) Resolve(tag language.Tag, msg Message) string {
	if msg.Code() == "" {
		return ""
	}
	if locale, ok := c.locales[tag]; ok {
		return render(locale[msg.Code()], msg.Meta())
	}
	return ""
}

func cloneLocales(locales map[language.Tag]map[string]string) map[language.Tag]map[string]string {
	if len(locales) == 0 {
		return map[language.Tag]map[string]string{}
	}
	out := make(map[language.Tag]map[string]string, len(locales))
	for tag, messages := range locales {
		cloned := make(map[string]string, len(messages))
		for code, text := range messages {
			cloned[code] = text
		}
		out[tag] = cloned
	}
	return out
}

func sortedTags(locales map[language.Tag]map[string]string) []language.Tag {
	tags := make([]language.Tag, 0, len(locales))
	for tag := range locales {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].String() < tags[j].String()
	})
	return tags
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
		return ""
	}
	var b bytes.Buffer
	if err := t.Execute(&b, meta); err != nil {
		return ""
	}
	return b.String()
}
