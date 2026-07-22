// Package errors provides structured, localizable application errors.
package errors

import (
	stdErrors "errors"
	"net/http"

	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// AppError is the source-layer error type returned by business, store, and
// infrastructure packages. It carries a stable typed message plus metadata,
// while the transport layer turns it into a localized response.
type AppError struct {
	status int
	msg    i18n.Message
	meta   map[string]any
	cause  error
	logged bool
}

// Option configures an AppError.
type Option func(*AppError)

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

// Logged reports whether a boundary has already recorded this error.
func (e *AppError) Logged() bool {
	return e.logged
}

// MarkLogged records that a boundary has emitted this error so later
// boundaries do not log it again.
func (e *AppError) MarkLogged() {
	e.logged = true
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return string(e.Code())
}

// Unwrap returns the underlying cause, if one was attached.
func (e *AppError) Unwrap() error {
	return e.cause
}

// Is matches another AppError by code so errors.Is works across wrapped values.
func (e *AppError) Is(target error) bool {
	var appErr *AppError
	if !stdErrors.As(target, &appErr) {
		return false
	}
	return e.Code() == appErr.Code()
}

// Status returns the HTTP status associated with the error.
func (e *AppError) Status() int {
	return e.status
}

// Code returns the stable machine-readable error code.
func (e *AppError) Code() i18n.Code {
	return e.msg.Code()
}

// Meta returns a defensive copy of the metadata associated with the error.
func (e *AppError) Meta() map[string]any {
	return mergeMeta(e.msg.Meta(), e.meta)
}

// Message returns the localized message descriptor associated with the error.
func (e *AppError) Message() i18n.Message {
	return e.msg
}
