package transform

import (
	"net/url"
	"strings"
)

func Email(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func Trim(value string) string {
	return strings.TrimSpace(value)
}

func InternalReturnPath(raw string) string {
	if raw == "" {
		return "/"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.IsAbs() || !strings.HasPrefix(parsed.Path, "/") {
		return "/"
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.RequestURI()
}
