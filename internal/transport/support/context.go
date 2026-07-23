package support

import (
	"context"
	"net/http"

	"golang.org/x/text/language"
)

type ctxKey int

const (
	ctxKeySecure ctxKey = iota
	ctxKeyLang
	ctxKeyCookies
)

// WithSecure marks whether the current request arrived over a secure (HTTPS) connection.
func WithSecure(ctx context.Context, secure bool) context.Context {
	return context.WithValue(ctx, ctxKeySecure, secure)
}

// IsSecure reports whether the current request arrived over HTTPS (direct TLS or trusted proxy).
func IsSecure(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySecure).(bool)
	return v
}

// WithRequestCookies attaches request cookie values to the context.
func WithRequestCookies(ctx context.Context, cookies []*http.Cookie) context.Context {
	values := make(map[string]string, len(cookies))
	for _, cookie := range cookies {
		values[cookie.Name] = cookie.Value
	}
	return context.WithValue(ctx, ctxKeyCookies, values)
}

// CookieValueFrom returns the cookie value for name from the request context.
func CookieValueFrom(ctx context.Context, name string) string {
	values, _ := ctx.Value(ctxKeyCookies).(map[string]string)
	return values[name]
}

// WithLang attaches the resolved language tag to the request context.
func WithLang(ctx context.Context, tag language.Tag) context.Context {
	return context.WithValue(ctx, ctxKeyLang, tag)
}

// LangFrom returns the language tag stored by the Lang middleware.
// Returns language.English if no tag has been set.
func LangFrom(ctx context.Context) language.Tag {
	if tag, ok := ctx.Value(ctxKeyLang).(language.Tag); ok {
		return tag
	}
	return language.English
}
