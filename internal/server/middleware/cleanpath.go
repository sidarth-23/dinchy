package middleware

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// CleanPath normalizes double slashes in request paths (e.g. /users//1 → /users/1).
func CleanPath() func(http.Handler) http.Handler {
	return chimw.CleanPath
}
