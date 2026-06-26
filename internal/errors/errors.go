package errors

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/text/language"

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

// WithMetaMap copies the provided metadata map into the error.
func WithMetaMap(meta map[string]any) Option {
	return func(e *AppError) {
		if len(meta) == 0 {
			return
		}
		if e.meta == nil {
			e.meta = make(map[string]any, len(meta))
		}
		for k, v := range meta {
			e.meta[k] = v
		}
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
	var appErr *AppError
	if stdErrors.As(err, &appErr) {
		merged := []Option{WithCause(appErr.cause), WithMetaMap(appErr.meta)}
		merged = append(merged, opts...)
		return New(appErr.status, appErr.msg, merged...)
	}
	opts = append([]Option{WithCause(err)}, opts...)
	return Internal(i18n.Msg(i18n.CodeServerInternalError), opts...)
}

// ResponsePayload is the public error payload serialized by transport.
type ResponsePayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Meta    map[string]any `json:"meta,omitempty"`
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

// ResponseFor converts any error into a localized transport error response.
// If the error already is an AppError, the code and metadata are preserved.
// Otherwise a generic internal error is returned.
func ResponseFor(tag language.Tag, catalog *i18n.Catalog, status int, errs ...error) *ErrorResponse {
	if appErr := findAppError(errs...); appErr != nil {
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

func localizedResponse(tag language.Tag, catalog *i18n.Catalog, err *AppError) *ErrorResponse {
	meta := err.Meta()
	return &ErrorResponse{
		status: err.status,
		Payload: ResponsePayload{
			Code:    string(err.Code()),
			Message: catalog.Resolve(tag, err.Message()),
			Meta:    meta,
		},
	}
}

func validationResponse(tag language.Tag, catalog *i18n.Catalog, errs ...error) *ErrorResponse {
	meta := map[string]any{}
	if fields := validationDetails(errs...); len(fields) > 0 {
		meta["fields"] = fields
	}
	return &ErrorResponse{
		status: http.StatusUnprocessableEntity,
		Payload: ResponsePayload{
			Code:    string(i18n.CodeRequestValidationFailed),
			Message: catalog.Resolve(tag, i18n.Msg(i18n.CodeRequestValidationFailed)),
			Meta:    meta,
		},
	}
}

func validationDetails(errs ...error) []map[string]any {
	var details []map[string]any
	for _, err := range errs {
		var detailer huma.ErrorDetailer
		if stdErrors.As(err, &detailer) {
			detail := detailer.ErrorDetail()
			if detail == nil {
				continue
			}
			item := map[string]any{
				"message": detail.Message,
			}
			if detail.Location != "" {
				item["location"] = detail.Location
			}
			if detail.Value != nil {
				item["value"] = detail.Value
			}
			details = append(details, item)
			continue
		}
		if err != nil {
			details = append(details, map[string]any{
				"message": err.Error(),
			})
		}
	}
	return details
}

func findAppError(errs ...error) *AppError {
	for _, err := range errs {
		if err == nil {
			continue
		}
		var appErr *AppError
		if stdErrors.As(err, &appErr) {
			return appErr
		}
	}
	return nil
}

func mergeMeta(base, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
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
	var appErr *AppError
	if stdErrors.As(err, &appErr) {
		return localizedResponse(tag, catalog, appErr)
	}
	return localizedResponse(tag, catalog, Internal(i18n.Msg(i18n.CodeServerInternalError), WithCause(err)))
}
