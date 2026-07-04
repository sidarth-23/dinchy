package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			attrs := []any{
				slog.String("request_id", support.RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_ip", support.RemoteIPFrom(r.Context())),
			}
			if session := auth.SessionFrom(r.Context()); session != nil {
				attrs = append(attrs, slog.String("actor_user_id", session.UserID), slog.String("actor_organisation_id", session.OrganisationID))
			}
			requestLogger := logger.With(attrs...)
			ctx := logging.WithContext(r.Context(), requestLogger)
			next.ServeHTTP(ww, r.WithContext(ctx))
			requestLogger.InfoContext(ctx, "HTTP request completed",
				slog.Int("status", ww.Status()),
				slog.Duration("duration", time.Since(startedAt)),
			)
		})
	}
}
