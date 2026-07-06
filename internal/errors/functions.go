package errors

//go:generate go run ../../cmd/codegen error -input catalog.json -output generated.go

import (
	"maps"
	"net/http"

	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

// WithMetaMap copies the provided metadata map into the error.
func WithMetaMap(meta map[string]any) Option {
	return func(e *AppError) {
		if len(meta) == 0 {
			return
		}
		if e.meta == nil {
			e.meta = make(map[string]any, len(meta))
		}
		maps.Copy(e.meta, meta)
	}
}

// WithCause sets the wrapped cause on the error.
func WithCause(err error) Option {
	return func(e *AppError) {
		e.cause = err
	}
}

// New creates a new source-layer app error.
func New(status int, msg i18n.Message, opts ...Option) *AppError {
	e := &AppError{status: status, msg: msg}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// BadRequest creates a 400 AppError.
func BadRequest(msg i18n.Message, opts ...Option) *AppError {
	return New(http.StatusBadRequest, msg, opts...)
}

// Unauthorized creates a 401 AppError.
func Unauthorized(msg i18n.Message, opts ...Option) *AppError {
	return New(http.StatusUnauthorized, msg, opts...)
}

// Forbidden creates a 403 AppError.
func Forbidden(msg i18n.Message, opts ...Option) *AppError {
	return New(http.StatusForbidden, msg, opts...)
}

// Conflict creates a 409 AppError.
func Conflict(msg i18n.Message, opts ...Option) *AppError {
	return New(http.StatusConflict, msg, opts...)
}

// TooManyRequests creates a 429 AppError.
func TooManyRequests(msg i18n.Message, opts ...Option) *AppError {
	return New(http.StatusTooManyRequests, msg, opts...)
}

// UnprocessableEntity creates a 422 AppError.
func UnprocessableEntity(msg i18n.Message, opts ...Option) *AppError {
	return New(http.StatusUnprocessableEntity, msg, opts...)
}

// Internal creates a 500 AppError.
func Internal(msg i18n.Message, opts ...Option) *AppError {
	return New(http.StatusInternalServerError, msg, opts...)
}

// Annotate preserves an existing structured error and adds more metadata.
// If err is not already structured, it becomes a catch-all internal error.
func Annotate(err error, opts ...Option) error {
	if err == nil {
		return nil
	}
	if appErr, ok := appErrorFrom(err); ok {
		merged := []Option{WithCause(appErr.cause), WithMetaMap(appErr.meta)}
		merged = append(merged, opts...)
		return New(appErr.status, appErr.msg, merged...)
	}
	opts = append([]Option{WithCause(err)}, opts...)
	return Internal(i18n.Msg(i18n.CodeServerInternalError), opts...)
}

// ResponseFor converts any error into a localized transport error response.
// If the error already is an AppError, the code and metadata are preserved.
// Otherwise a generic internal error is returned.
func ResponseFor(tag language.Tag, catalog *i18n.Catalog, status int, errs ...error) *ErrorResponse {
	if appErr := firstAppError(errs...); appErr != nil {
		return localizedResponse(tag, catalog, appErr)
	}

	if status == http.StatusUnprocessableEntity {
		return validationResponse(tag, catalog, errs...)
	}

	return &ErrorResponse{
		status: status,
		Payload: ResponsePayload{
			Code:    string(i18n.CodeServerInternalError),
			Message: catalog.Resolve(tag, i18n.Msg(i18n.CodeServerInternalError)),
		},
	}
}

// Render resolves a localized message for an application error.
func Render(tag language.Tag, catalog *i18n.Catalog, err *AppError) ResponsePayload {
	return ResponsePayload{
		Code:    string(err.Code()),
		Message: catalog.Resolve(tag, err.Message()),
		Meta:    err.Meta(),
	}
}

// Resolve returns a localized response payload for a source-layer error or
// a generic internal error if the error is not recognized.
func Resolve(tag language.Tag, catalog *i18n.Catalog, err error) *ErrorResponse {
	if err == nil {
		return nil
	}
	if appErr, ok := appErrorFrom(err); ok {
		return localizedResponse(tag, catalog, appErr)
	}
	return localizedResponse(tag, catalog, Internal(i18n.Msg(i18n.CodeServerInternalError), WithCause(err)))
}
