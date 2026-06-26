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
// infrastructure packages. It carries a stable code plus metadata, while the
// transport layer turns it into a localized response.
type AppError struct {
	status int
	code   string
	meta   map[string]any
	cause  error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	return e.code
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
	return e.code == appErr.code
}

// Status returns the HTTP status associated with the error.
func (e *AppError) Status() int {
	return e.status
}

// Code returns the stable machine-readable error code.
func (e *AppError) Code() string {
	return e.code
}

// Meta returns a defensive copy of the metadata associated with the error.
func (e *AppError) Meta() map[string]any {
	return cloneMeta(e.meta)
}

// Option configures an AppError.
type Option func(*AppError)

// WithMeta adds a single metadata entry.
func WithMeta(key string, value any) Option {
	return func(e *AppError) {
		if e.meta == nil {
			e.meta = make(map[string]any)
		}
		e.meta[key] = value
	}
}

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
func New(status int, code string, opts ...Option) *AppError {
	e := &AppError{status: status, code: code}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// InvalidCredentials returns the canonical auth failure for a bad login.
func InvalidCredentials() *AppError {
	return New(http.StatusUnauthorized, CodeAuthInvalidCredentials)
}

// SetupCompleted returns the canonical auth failure when first-user setup has already happened.
func SetupCompleted(opts ...Option) *AppError {
	return New(http.StatusConflict, CodeAuthSetupCompleted, opts...)
}

// Unauthenticated returns the canonical auth failure for unauthenticated requests.
func Unauthenticated() *AppError {
	return New(http.StatusUnauthorized, CodeAuthUnauthenticated)
}

// HTTPSRequired returns the canonical security failure for insecure auth requests.
func HTTPSRequired() *AppError {
	return New(http.StatusForbidden, CodeSecurityHTTPSRequired)
}

// CSRFFailed returns the canonical security failure for missing or invalid CSRF tokens.
func CSRFFailed() *AppError {
	return New(http.StatusBadRequest, CodeSecurityCSRFFailed)
}

// ValidationFailed returns the canonical validation error.
func ValidationFailed(opts ...Option) *AppError {
	return New(http.StatusUnprocessableEntity, CodeRequestValidationFailed, opts...)
}

// ConfigLoadFailed returns a startup/configuration error.
func ConfigLoadFailed(cause error, opts ...Option) *AppError {
	opts = append([]Option{WithCause(cause)}, opts...)
	return New(http.StatusInternalServerError, CodeConfigLoadFailed, opts...)
}

// ConfigValidationFailed returns a startup/configuration validation error.
func ConfigValidationFailed(cause error, opts ...Option) *AppError {
	opts = append([]Option{WithCause(cause)}, opts...)
	return New(http.StatusInternalServerError, CodeConfigValidationFailed, opts...)
}

// Internal returns the canonical catch-all server error.
func Internal(cause error, opts ...Option) *AppError {
	opts = append([]Option{WithCause(cause)}, opts...)
	return New(http.StatusInternalServerError, CodeServerInternalError, opts...)
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
func ResponseFor(tag language.Tag, catalog *i18n.Catalog, status int, msg string, errs ...error) *ErrorResponse {
	if appErr := findAppError(errs...); appErr != nil {
		return localizedResponse(tag, catalog, appErr)
	}

	if status == http.StatusUnprocessableEntity {
		return validationResponse(tag, catalog, errs...)
	}

	if appErr := findAppError(stdErrors.Join(errs...)); appErr != nil {
		return localizedResponse(tag, catalog, appErr)
	}

	code := codeFor(status, msg)
	if code == "" {
		code = CodeServerInternalError
		status = http.StatusInternalServerError
	}
	return &ErrorResponse{
		status: status,
		Payload: ResponsePayload{
			Code:    code,
			Message: catalog.Resolve(tag, code, nil),
		},
	}
}

func localizedResponse(tag language.Tag, catalog *i18n.Catalog, err *AppError) *ErrorResponse {
	meta := err.Meta()
	return &ErrorResponse{
		status: err.status,
		Payload: ResponsePayload{
			Code:    err.code,
			Message: catalog.Resolve(tag, err.code, meta),
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
			Code:    CodeRequestValidationFailed,
			Message: catalog.Resolve(tag, CodeRequestValidationFailed, meta),
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

func codeFor(status int, msg string) string {
	switch status {
	case http.StatusUnauthorized:
		switch msg {
		case "authentication required":
			return CodeAuthUnauthenticated
		}
	case http.StatusForbidden:
		switch msg {
		case "this endpoint requires a secure (HTTPS) connection":
			return CodeSecurityHTTPSRequired
		}
	case http.StatusBadRequest:
		switch msg {
		case "missing or invalid CSRF token":
			return CodeSecurityCSRFFailed
		}
	case http.StatusUnprocessableEntity:
		return CodeRequestValidationFailed
	case http.StatusConflict:
		switch msg {
		case "setup has already been completed":
			return CodeAuthSetupCompleted
		}
	}
	return CodeServerInternalError
}

func cloneMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}

// Render resolves a localized message for an application error.
func Render(tag language.Tag, catalog *i18n.Catalog, err *AppError) ResponsePayload {
	return ResponsePayload{
		Code:    err.code,
		Message: catalog.Resolve(tag, err.code, err.Meta()),
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
	return localizedResponse(tag, catalog, Internal(err))
}
