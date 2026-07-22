package errors_test

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

func TestAnnotatePreservesCodeAndAddsMeta(t *testing.T) {
	t.Parallel()

	base := apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 2)))
	base.MarkLogged()
	err := apperrors.Annotate(base, apperrors.WithFieldName(apperrors.FieldName("email")))

	var got *apperrors.AppError
	require.ErrorAs(t, err, &got)
	assert.Equal(t, i18n.CodeAccountAuthSetupCompleted, got.Code())
	assert.Equal(t, "users", got.Meta()["resource"])
	assert.Equal(t, 2, got.Meta()["count"])
	assert.Equal(t, "email", got.Meta()["field_name"])
	assert.True(t, got.Logged())
	assert.True(t, stdErrors.Is(err, apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 2)))))
}

func TestAnnotateWrapsUnstructuredAsInternal(t *testing.T) {
	t.Parallel()

	err := apperrors.Annotate(assert.AnError)

	var got *apperrors.AppError
	require.ErrorAs(t, err, &got)
	assert.Equal(t, i18n.CodePlatformServerInternalError, got.Code())
	assert.ErrorIs(t, err, assert.AnError)
}
