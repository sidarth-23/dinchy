package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/sidarth-23/dinchy/internal/foundation/requestcontext"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
	"github.com/sidarth-23/dinchy/internal/transport/render"
	"github.com/sidarth-23/dinchy/internal/transport/support"
)

// Recover catches handler panics, logs them once through the structured
// logging pipeline, and returns the standard 500 error envelope when the
// response has not started yet.
func Recover(logger *slog.Logger, renderer *render.Renderer) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}

				ctx := r.Context()
				logging.Panic(
					ctx, logger, "Recovered handler panic", recovered, debug.Stack(),
					slog.String("request_id", requestcontext.RequestIDFrom(ctx)),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
				)
				if ww.Status() == 0 {
					writeErrorResponse(ctx, ww, logger, renderer.ResponseFor(support.LangFrom(ctx), http.StatusInternalServerError))
				}
			}()
			next.ServeHTTP(ww, r)
		})
	}
}

func writeErrorResponse(ctx context.Context, w http.ResponseWriter, logger *slog.Logger, response *render.ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.GetStatus())
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logging.Error(ctx, logger, "Encode error response", err)
	}
}
