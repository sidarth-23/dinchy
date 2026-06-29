package middleware

import (
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Timeout sets a deadline on the request context. Handlers should respect ctx.Done().
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return chimw.Timeout(d)
}
