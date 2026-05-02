package support

import (
	"context"

	"github.com/sidarth-23/dinchy/internal/domain"
)

type ctxKey int

const (
	ctxKeySession ctxKey = iota
	ctxKeySecure
	ctxKeyRemoteIP
	ctxKeyUserAgent
)

// WithSession attaches a validated session to the request context.
func WithSession(ctx context.Context, s *domain.SessionWithUser) context.Context {
	return context.WithValue(ctx, ctxKeySession, s)
}

// SessionFrom retrieves the session from the context, or nil for anonymous requests.
func SessionFrom(ctx context.Context) *domain.SessionWithUser {
	s, _ := ctx.Value(ctxKeySession).(*domain.SessionWithUser)
	return s
}

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
