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
	return &Validator{validate: validator.New(validator.WithRequiredStructEnabled())}
}

func (v *Validator) Struct(value any) error {
	if err := v.validate.Struct(value); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(err))
	}
	return nil
}

func (v *Validator) RegisterValidation(tag string, fn validator.Func, callValidationEvenIfNull ...bool) error {
	return v.validate.RegisterValidation(tag, fn, callValidationEvenIfNull...)
}

func (v *Validator) RegisterStructValidation(fn validator.StructLevelFunc, types ...any) {
	v.validate.RegisterStructValidation(fn, types...)
}
