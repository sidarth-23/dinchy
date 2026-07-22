package config

import (
	"github.com/go-playground/validator/v10"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// validateStruct validates value against its go-playground validator tags and
// returns an internal AppError when any rule fails.
func validateStruct(value any) error {
	v := validator.New(validator.WithRequiredStructEnabled())
	if err := v.Struct(value); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodePlatformConfigValidationFailed), apperrors.WithCause(err))
	}
	return nil
}
