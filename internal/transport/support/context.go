package support

import (
	"context"
	"net/http"

	"golang.org/x/text/language"
)

type ctxKey int

const (
	ctxKeyLang ctxKey = iota
	ctxKeyCookies
	ctxKeySecureCookies
)

// WithSecureCookies records whether cookies must carry the Secure attribute.
func WithSecureCookies(ctx context.Context, secure bool) context.Context {
	return context.WithValue(ctx, ctxKeySecureCookies, secure)
}

// SecureCookies reports whether cookies must carry the Secure attribute.
//
// This is deliberately a deployment-wide setting rather than something inferred from the
// current request. Cookies are not scoped by port or scheme, so a cookie minted without
// Secure over plaintext on one port overwrites the Secure cookie of the same name from
// HTTPS on another — which presents as being logged out at random. Caddy terminates TLS
// for every route, so the answer is a property of the installation, not of one request.
func SecureCookies(ctx context.Context) bool {
	v, _ := ctx.Value(ctxKeySecureCookies).(bool)
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
