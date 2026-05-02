package apierr

import (
	"context"
	"errors"

	"github.com/sidarth-23/dinchy/internal/auth"
)

// MapServiceError translates known domain errors from service calls into localized
// API errors. Unknown errors map to ErrInternal.
func MapServiceError(ctx context.Context, err error) *LocalizedError {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return Localized(ctx, ErrInvalidCredentials())
	case errors.Is(err, auth.ErrSetupCompleted):
		return Localized(ctx, ErrSetupCompleted())
	default:
		return Localized(ctx, ErrInternal())
	}
}
