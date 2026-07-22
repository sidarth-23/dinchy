// Package validation validates structs against go-playground validator tags.
package validation

import (
	"github.com/go-playground/validator/v10"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// Validator validates values using registered struct and field rules.
type Validator struct {
	validate *validator.Validate
}

// New creates a Validator with required-struct validation enabled.
func New() *Validator {
	return &Validator{validate: validator.New(validator.WithRequiredStructEnabled())}
}

// Struct validates value and returns an internal AppError when any rule fails.
func (v *Validator) Struct(value any) error {
	if err := v.validate.Struct(value); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodePlatformConfigValidationFailed), apperrors.WithCause(err))
	}
	return nil
}

// RegisterValidation registers a custom field-level validation under tag.
func (v *Validator) RegisterValidation(tag string, fn validator.Func, callValidationEvenIfNull ...bool) error {
	return v.validate.RegisterValidation(tag, fn, callValidationEvenIfNull...)
}

// RegisterStructValidation registers a struct-level validation for the given types.
func (v *Validator) RegisterStructValidation(fn validator.StructLevelFunc, types ...any) {
	v.validate.RegisterStructValidation(fn, types...)
}
