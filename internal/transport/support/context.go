package support

import (
	"context"

	"golang.org/x/text/language"
)

type ctxKey int

const (
	ctxKeySecure ctxKey = iota
	ctxKeyRemoteIP
	ctxKeyUserAgent
	ctxKeyLang
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

// WithRequestInfo injects the client IP address and User-Agent into the context.
func WithRequestInfo(ctx context.Context, ip, ua string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyRemoteIP, ip)
	ctx = context.WithValue(ctx, ctxKeyUserAgent, ua)
	return ctx
}

// RemoteIPFrom retrieves the client IP address injected by the request-info middleware.
func RemoteIPFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyRemoteIP).(string)
	return s
}

// UserAgentFrom retrieves the User-Agent header value injected by the request-info middleware.
func UserAgentFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxKeyUserAgent).(string)
	return s
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
