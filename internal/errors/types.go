package errors

import (
	"encoding/json"
	stdErrors "errors"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

// AppError is the source-layer error type returned by business, store, and
// infrastructure packages. It carries a stable typed message plus metadata,
// while the transport layer turns it into a localized response.
type AppError struct {
	status int
	msg    i18n.Message
	meta   map[string]any
	cause  error
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

// Option configures an AppError.
type Option func(*AppError)

// ResponsePayload is the public error payload serialized by transport.
type ResponsePayload struct {
	Code    string         `json:"code" doc:"Stable machine-readable error code"`
	Message string         `json:"message" doc:"Localized, human-readable error message"`
	Meta    map[string]any `json:"meta,omitempty" doc:"Additional error context; for validation failures this carries a fields array of {message, location, value}"`
}

// ErrorResponse is the transport-layer error type returned to Huma.
type ErrorResponse struct {
	status  int
	Payload ResponsePayload `json:"error"`
}

// Error implements the error interface.
func (e *ErrorResponse) Error() string {
	return e.Payload.Code
}

// GetStatus implements huma.StatusError.
func (e *ErrorResponse) GetStatus() int {
	return e.status
}

// MarshalJSON serializes the response as {"error":{...}}.
func (e *ErrorResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Error ResponsePayload `json:"error"`
	}{
		Error: e.Payload,
	})
}
