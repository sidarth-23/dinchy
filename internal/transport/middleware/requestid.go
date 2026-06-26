// Package middleware provides HTTP middleware for the Dinchy server.
// All middleware use the standard func(http.Handler) http.Handler signature,
// making them router-agnostic. Each middleware has its own file as the single
// source of truth — swap the backing implementation here without touching server.go.
package middleware

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestID injects a unique request ID into each request context and response header.
func RequestID() func(http.Handler) http.Handler {
	return chimw.RequestID
}
