package errors

import (
	stdErrors "errors"
	"maps"

	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

type metaValue interface {
	metaKey() MetaKey
	metaValue() any
}

func withMetaValue(v metaValue) Option {
	return func(e *AppError) {
		if e.meta == nil {
			e.meta = make(map[string]any)
		}
		e.meta[string(v.metaKey())] = v.metaValue()
	}
}

func appErrorFrom(err error) (*AppError, bool) {
	var appErr *AppError
	if !stdErrors.As(err, &appErr) {
		return nil, false
	}
	return appErr, true
}

func mergeMeta(base, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(extra))
	maps.Copy(out, base)
	maps.Copy(out, extra)
	return out
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
