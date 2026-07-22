package manifest

import (
	"strings"
	"unicode"
)

var goAcronyms = map[string]string{
	"csrf":  "CSRF",
	"http":  "HTTP",
	"https": "HTTPS",
	"id":    "ID",
	"oidc":  "OIDC",
	"sql":   "SQL",
	"sso":   "SSO",
	"totp":  "TOTP",
	"url":   "URL",
	"xdg":   "XDG",
}

// GoName converts a snake_case or dot-separated manifest segment into a Go export name.
func GoName(value string) string {
	segments := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_' || r == '.' || r == '/'
	})
	var b strings.Builder
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		if acronym, ok := goAcronyms[strings.ToLower(segment)]; ok {
			b.WriteString(acronym)
			continue
		}
		if segment == strings.ToUpper(segment) {
			b.WriteString(segment)
			continue
		}
		b.WriteString(strings.ToUpper(segment[:1]))
		b.WriteString(strings.ToLower(segment[1:]))
	}
	return b.String()
}

// GoNameFromPath converts a path of manifest segments into a Go export name.
func GoNameFromPath(segments ...string) string {
	return GoName(strings.Join(segments, "_"))
}

// LowerCamel lowercases the leading rune of a Go identifier, returning "value"
// for the empty string. It is used to derive receiver and parameter names.
func LowerCamel(name string) string {
	if name == "" {
		return "value"
	}
	runes := []rune(name)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// DisplayPath renders a path for diagnostics.
func DisplayPath(parts []string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		cleaned = append(cleaned, part)
	}
	if len(cleaned) == 0 {
		return "<root>"
	}
	return strings.Join(cleaned, ".")
}
