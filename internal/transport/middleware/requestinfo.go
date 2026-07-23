package middleware

import (
	"net"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/foundation/requestmeta"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// RequestInfo injects the resolved client IP address and User-Agent into the request context.
// Relies on RealIP middleware having already resolved r.RemoteAddr from proxy headers.
func RequestInfo() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			if host, _, err := net.SplitHostPort(ip); err == nil {
				ip = host
			}
			ctx := requestmeta.WithRequestInfo(r.Context(), ip, r.UserAgent())
			ctx = support.WithRequestCookies(ctx, r.Cookies())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
