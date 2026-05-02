package middleware

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Recover catches panics, logs the backtrace, and returns HTTP 500.
func Recover() func(http.Handler) http.Handler {
	return chimw.Recoverer
}
