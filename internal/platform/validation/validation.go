package validation

import (
	"github.com/go-playground/validator/v10"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

type Validator struct {
	validate *validator.Validate
}

func New() *Validator {
	return &Validator{validate: validator.New()}
}

func (v *Validator) Struct(value any) error {
	if err := v.validate.Struct(value); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(err))
	}
	return nil
}
