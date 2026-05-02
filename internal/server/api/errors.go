// Package api contains the Huma API handlers and operation registrations for Dinchy.
package api

import (
	"errors"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/auth"
)

// DinchyError is the structured error envelope returned by all API endpoints.
type DinchyError struct {
	status  int
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *DinchyError) Error() string { return e.Message }

// GetStatus implements huma.StatusError, telling huma which HTTP status to use.
func (e *DinchyError) GetStatus() int { return e.status }

// ErrInvalidCredentials returns a 401 for failed login attempts.
func ErrInvalidCredentials() *DinchyError {
	return &DinchyError{status: http.StatusUnauthorized, Code: "auth.invalid_credentials", Message: "Invalid email or password."}
}

// ErrSetupCompleted returns a 409 when first-user setup has already been done.
func ErrSetupCompleted() *DinchyError {
	return &DinchyError{status: http.StatusConflict, Code: "auth.setup_completed", Message: "Setup has already been completed."}
}

// ErrUnauthenticated returns a 401 for requests that require authentication.
func ErrUnauthenticated() *DinchyError {
	return &DinchyError{status: http.StatusUnauthorized, Code: "auth.unauthenticated", Message: "Authentication required."}
}

// ErrHTTPSRequired returns a 403 when the HTTPS-required policy is active and the request is insecure.
func ErrHTTPSRequired() *DinchyError {
	return &DinchyError{status: http.StatusForbidden, Code: "security.https_required", Message: "This endpoint requires a secure (HTTPS) connection."}
}

// ErrInternal returns a 500 for unexpected server errors.
func ErrInternal() *DinchyError {
	return &DinchyError{status: http.StatusInternalServerError, Code: "server.internal_error", Message: "An unexpected error occurred."}
}

// MapServiceError translates known domain errors from service calls into structured API errors.
func MapServiceError(err error) *DinchyError {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return ErrInvalidCredentials()
	case errors.Is(err, auth.ErrSetupCompleted):
		return ErrSetupCompleted()
	default:
		return ErrInternal()
	}
}
