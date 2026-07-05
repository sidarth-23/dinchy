package manifest

import "strings"

var goAcronyms = map[string]string{
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
		b.WriteString(goSegment(segment))
	}
	return b.String()
}

// GoNameFromPath converts a path of manifest segments into a Go export name.
func GoNameFromPath(segments ...string) string {
	return GoName(strings.Join(segments, "_"))
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

func goSegment(segment string) string {
	if segment == "" {
		return ""
	}
	if acronym, ok := goAcronyms[strings.ToLower(segment)]; ok {
		return acronym
	}
	if segment == strings.ToUpper(segment) {
		return segment
	}
	return strings.ToUpper(segment[:1]) + strings.ToLower(segment[1:])
}
