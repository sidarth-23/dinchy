// Package requestmeta carries request-scoped observability values — client IP,
// user agent, and request, trace, and span IDs — across layers via the context.
// Transport middleware populates it; any layer that needs request provenance
// reads it, including code outside the transport layer.
package requestmeta

import (
	"context"

	chimw "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
)

type ctxKey int

const (
	ctxKeyRemoteIP ctxKey = iota
	ctxKeyUserAgent
)

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

// RequestIDFrom returns the request ID assigned by the chi request-ID middleware.
func RequestIDFrom(ctx context.Context) string {
	return chimw.GetReqID(ctx)
}

// TraceIDFrom returns the current trace ID, or empty when no valid span is present.
func TraceIDFrom(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.TraceID().String()
}

// SpanIDFrom returns the current span ID, or empty when no valid span is present.
func SpanIDFrom(ctx context.Context) string {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return ""
	}
	return spanContext.SpanID().String()
}
