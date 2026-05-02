// Package apierr defines the structured error types returned by Dinchy's HTTP API.
// It is a separate package (not inside api/) so that middleware packages can use
// these types without creating an import cycle.
package apierr

import (
	"context"
	"net/http"

	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

// DinchyError is a language-neutral structured error carrying an HTTP status code
// and an error code. The human-readable message is resolved lazily at response time
// via Localized, allowing the same error value to be localized into any language.
type DinchyError struct {
	status int
	Code   string
}

// Error implements the error interface.
func (e *DinchyError) Error() string { return e.Code }

// GetStatus implements huma.StatusError.
func (e *DinchyError) GetStatus() int { return e.status }

// LocalizedError is DinchyError with a resolved human-readable message.
// This is the type serialized to JSON in HTTP responses.
type LocalizedError struct {
	status  int
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *LocalizedError) Error() string { return e.Message }

// GetStatus implements huma.StatusError.
func (e *LocalizedError) GetStatus() int { return e.status }

// Localized resolves the error's message from the catalog using the language tag
// stored in ctx by the Lang middleware. Call this at handler return sites:
//
//	return nil, apierr.Localized(ctx, apierr.ErrFoo())
func Localized(ctx context.Context, err *DinchyError) *LocalizedError {
	tag := support.LangFrom(ctx)
	return &LocalizedError{
		status:  err.status,
		Code:    err.Code,
		Message: i18n.Default.Resolve(tag, err.Code),
	}
}

// LocalizedTag resolves the error using an explicit language tag instead of the context.
func LocalizedTag(tag language.Tag, err *DinchyError) *LocalizedError {
	return &LocalizedError{
		status:  err.status,
		Code:    err.Code,
		Message: i18n.Default.Resolve(tag, err.Code),
	}
}

// ErrInvalidCredentials returns a 401 for failed login attempts.
func ErrInvalidCredentials() *DinchyError {
	return &DinchyError{status: http.StatusUnauthorized, Code: "auth.invalid_credentials"}
}

// ErrSetupCompleted returns a 409 when first-user setup has already been done.
func ErrSetupCompleted() *DinchyError {
	return &DinchyError{status: http.StatusConflict, Code: "auth.setup_completed"}
}

// ErrUnauthenticated returns a 401 for requests that require authentication.
func ErrUnauthenticated() *DinchyError {
	return &DinchyError{status: http.StatusUnauthorized, Code: "auth.unauthenticated"}
}

// ErrHTTPSRequired returns a 403 when HTTPS is required but the request is insecure.
func ErrHTTPSRequired() *DinchyError {
	return &DinchyError{status: http.StatusForbidden, Code: "security.https_required"}
}

// ErrCSRFFailed returns a 400 for requests with a missing or invalid CSRF token.
func ErrCSRFFailed() *DinchyError {
	return &DinchyError{status: http.StatusBadRequest, Code: "security.csrf_failed"}
}

// ErrInternal returns a 500 for unexpected server errors.
func ErrInternal() *DinchyError {
	return &DinchyError{status: http.StatusInternalServerError, Code: "server.internal_error"}
}
