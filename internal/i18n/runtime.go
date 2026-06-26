package i18n

//go:generate go run ../../cmd/gen i18n -input messages.json -output generated.go

import (
	"bytes"
	"sort"
	"text/template"

	"golang.org/x/text/language"
)

// Message carries a machine-readable code and optional interpolation metadata.
type Message struct {
	code Code
	meta map[string]any
}

// Code returns the stable identifier for the message.
func (m Message) Code() Code {
	return m.code
}

// Meta returns a defensive copy of the message metadata.
func (m Message) Meta() map[string]any {
	return cloneMeta(m.meta)
}

// String returns the machine-readable code.
func (m Message) String() string {
	return string(m.code)
}

// Param represents one interpolation parameter for a message.
type Param struct {
	Key   string
	Value any
}

// P creates a typed interpolation parameter.
func P(key string, value any) Param {
	return Param{Key: key, Value: value}
}

// Msg builds a message descriptor from a code and optional params.
func Msg(code Code, params ...Param) Message {
	if len(params) == 0 {
		return Message{code: code}
	}
	meta := make(map[string]any, len(params))
	for _, p := range params {
		meta[p.Key] = p.Value
	}
	return Message{code: code, meta: cloneMeta(meta)}
}

// Catalog resolves message codes to localized templates.
type Catalog struct {
	locales map[language.Tag]map[Code]string
	tags    []language.Tag
	matcher language.Matcher
}

// New creates a catalog from locale data.
func New(locales map[language.Tag]map[Code]string) *Catalog {
	cloned := cloneLocales(locales)
	tags := sortedTags(cloned)
	return &Catalog{locales: cloned, tags: tags, matcher: language.NewMatcher(tags)}
}

// Match returns the best supported locale tag for an Accept-Language value.
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

// Resolve returns the localized message text for msg in tag.
func (c *Catalog) Resolve(tag language.Tag, msg Message) string {
	if msg.Code() == "" {
		return ""
	}
	if locale, ok := c.locales[tag]; ok {
		return render(locale[msg.Code()], msg.Meta())
	}
	return ""
}

func cloneLocales(locales map[language.Tag]map[Code]string) map[language.Tag]map[Code]string {
	if len(locales) == 0 {
		return map[language.Tag]map[Code]string{}
	}
	out := make(map[language.Tag]map[Code]string, len(locales))
	for tag, messages := range locales {
		cloned := make(map[Code]string, len(messages))
		for code, text := range messages {
			cloned[code] = text
		}
		out[tag] = cloned
	}
	return out
}

func cloneMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func sortedTags(locales map[language.Tag]map[Code]string) []language.Tag {
	tags := make([]language.Tag, 0, len(locales))
	for tag := range locales {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].String() < tags[j].String() })
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

// Default is the package-level catalog pre-loaded with generated translations.
var Default = New(CatalogData)
