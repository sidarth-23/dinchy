package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/sidarth-23/dinchy/internal/foundation/requestmeta"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// RequestInfo injects the resolved client IP address and User-Agent into the request context.
//
// The client address comes from X-Forwarded-For, which Caddy sets to the address of the
// peer that connected to it. Reading it here is not optional: the value is persisted to
// audit_logs.ip_address and sessions.ip_address, so using the connection's own address
// would record loopback for every request. Caddy replaces the header rather than appending
// to it, so there is no chain to walk and a client cannot forge the value.
func RequestInfo() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := requestmeta.WithRequestInfo(r.Context(), clientIP(r), r.UserAgent())
			ctx = support.WithRequestCookies(ctx, r.Cookies())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// clientIP resolves the address to record, falling back to the direct peer when the app is
// reached without a proxy in front of it.
func clientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if first, _, found := strings.Cut(forwarded, ","); found {
		forwarded = first
	}
	if address := strings.TrimSpace(forwarded); address != "" {
		return address
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
