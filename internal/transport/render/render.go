// Package render localizes application errors into transport-layer HTTP
// responses for Huma. It owns the client-facing error payload shape and whether
// internal failure detail is exposed; the source-layer error values it renders
// come from internal/errors.
package render

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/text/language"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// ResponsePayload is the public error payload serialized by transport.
type ResponsePayload struct {
	Code    string         `json:"code" doc:"Stable machine-readable error code"`
	Message string         `json:"message" doc:"Localized, human-readable error message"`
	Meta    map[string]any `json:"meta,omitempty" doc:"Additional error context; for validation failures this carries a fields array of {message, location, value}"`
	Debug   *DebugPayload  `json:"debug,omitempty" doc:"Internal diagnostics; present only when internal error exposure is enabled"`
}

// DebugPayload carries internal failure detail. It is populated only when the
// server is configured to expose internal errors and must never be relied upon
// in production, as it leaks internal codes, causes, and metadata.
type DebugPayload struct {
	Code  string         `json:"code,omitempty" doc:"Specific internal error code"`
	Cause string         `json:"cause,omitempty" doc:"Underlying cause chain, including backend errors"`
	Meta  map[string]any `json:"meta,omitempty" doc:"Internal error metadata"`
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

// Renderer localizes application errors into transport responses. It owns the
// message catalog and whether internal failure detail is exposed to clients.
type Renderer struct {
	catalog        *i18n.Catalog
	exposeInternal bool
}

// NewRenderer creates a Renderer bound to a catalog. When exposeInternal is
// true, every response carries a debug object with the internal code, cause,
// and metadata.
func NewRenderer(catalog *i18n.Catalog, exposeInternal bool) *Renderer {
	return &Renderer{catalog: catalog, exposeInternal: exposeInternal}
}

// ResponseFor converts any error into a localized transport error response.
// If an error is an AppError, its code and metadata are preserved; a validation
// status yields a field-detail payload; otherwise a generic internal error is returned.
func (r *Renderer) ResponseFor(tag language.Tag, status int, errs ...error) *ErrorResponse {
	for _, err := range errs {
		if err == nil {
			continue
		}
		if appErr, ok := appErrorFrom(err); ok {
			return r.localizedResponse(tag, appErr)
		}
	}

	if status == http.StatusUnprocessableEntity {
		meta := map[string]any{}
		if fields := validationDetails(errs...); len(fields) > 0 {
			meta["fields"] = fields
		}
		payload := ResponsePayload{
			Code:    string(i18n.CodeTransportRequestValidationFailed),
			Message: r.catalog.Resolve(tag, i18n.Msg(i18n.CodeTransportRequestValidationFailed)),
			Meta:    meta,
		}
		if r.exposeInternal {
			payload.Debug = debugForErrs(errs...)
		}
		return &ErrorResponse{status: http.StatusUnprocessableEntity, Payload: payload}
	}

	payload := ResponsePayload{
		Code:    string(i18n.CodePlatformServerInternalError),
		Message: r.catalog.Resolve(tag, i18n.Msg(i18n.CodePlatformServerInternalError)),
	}
	if r.exposeInternal {
		payload.Debug = debugForErrs(errs...)
	}
	return &ErrorResponse{status: status, Payload: payload}
}

// Resolve returns a localized response for a source-layer error, or a generic
// internal error when the error is not recognized.
func (r *Renderer) Resolve(tag language.Tag, err error) *ErrorResponse {
	if err == nil {
		return nil
	}
	if appErr, ok := appErrorFrom(err); ok {
		return r.localizedResponse(tag, appErr)
	}
	return r.localizedResponse(tag, apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(err)))
}

// localizedResponse builds the client response. Server errors (status >= 500)
// render the generic internal code and drop metadata so no internal detail
// reaches the client; the specific code and cause are surfaced only through the
// debug object when internal exposure is enabled.
func (r *Renderer) localizedResponse(tag language.Tag, appErr *apperrors.AppError) *ErrorResponse {
	var payload ResponsePayload
	if appErr.Status() >= http.StatusInternalServerError {
		payload = ResponsePayload{
			Code:    string(i18n.CodePlatformServerInternalError),
			Message: r.catalog.Resolve(tag, i18n.Msg(i18n.CodePlatformServerInternalError)),
		}
	} else {
		payload = ResponsePayload{
			Code:    string(appErr.Code()),
			Message: r.catalog.Resolve(tag, appErr.Message()),
			Meta:    appErr.Meta(),
		}
	}
	if r.exposeInternal {
		payload.Debug = debugForAppError(appErr)
	}
	return &ErrorResponse{status: appErr.Status(), Payload: payload}
}

func appErrorFrom(err error) (*apperrors.AppError, bool) {
	var appErr *apperrors.AppError
	if !stdErrors.As(err, &appErr) {
		return nil, false
	}
	return appErr, true
}

func debugForAppError(appErr *apperrors.AppError) *DebugPayload {
	debug := &DebugPayload{Code: string(appErr.Code()), Meta: appErr.Meta()}
	if cause := appErr.Unwrap(); cause != nil {
		debug.Cause = cause.Error()
	}
	return debug
}

func debugForErrs(errs ...error) *DebugPayload {
	for _, err := range errs {
		if err == nil {
			continue
		}
		return &DebugPayload{Cause: err.Error()}
	}
	return nil
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
