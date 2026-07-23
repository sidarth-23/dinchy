// Package middleware provides HTTP middleware for the transport layer.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/foundation/requestmeta"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

// AccessLog returns middleware that binds a request-scoped logger and logs each completed request.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			attrs := []any{
				slog.String("request_id", requestmeta.RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_ip", requestmeta.RemoteIPFrom(r.Context())),
			}
			if principal := session.PrincipalFrom(r.Context()); principal != nil {
				attrs = append(attrs, slog.String("actor_user_id", principal.UserID), slog.String("actor_organization_id", principal.OrganizationID))
			}
			requestLogger := logger.With(attrs...)
			ctx := logging.WithLogger(r.Context(), requestLogger)
			next.ServeHTTP(ww, r.WithContext(ctx))
			requestLogger.InfoContext(
				ctx, "HTTP request completed",
				slog.Int("status", ww.Status()),
				slog.Duration("duration", time.Since(startedAt)),
			)
		})
	}
}
