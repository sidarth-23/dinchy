package errors

import (
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// Annotate preserves an existing structured error and adds more metadata.
// If err is not already structured, it becomes a catch-all internal error.
func Annotate(err error, opts ...Option) error {
	if err == nil {
		return nil
	}
	if appErr, ok := appErrorFrom(err); ok {
		merged := []Option{WithCause(appErr.cause), WithMetaMap(appErr.meta)}
		merged = append(merged, opts...)
		annotated := New(appErr.status, appErr.msg, merged...)
		if appErr.logged {
			annotated.logged = true
		}
		return annotated
	}
	opts = append([]Option{WithCause(err)}, opts...)
	return Internal(i18n.Msg(i18n.CodeServerInternalError), opts...)
}
