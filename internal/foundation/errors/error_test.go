package errors_test

import (
	stdErrors "errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

func TestConstructors_StatusAndCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    *apperrors.AppError
		status int
		code   i18n.Code
	}{
		{"InvalidCredentials", apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidCredentials)), http.StatusUnauthorized, i18n.CodeAccountAuthInvalidCredentials},
		{"SetupCompleted", apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 3))), http.StatusConflict, i18n.CodeAccountAuthSetupCompleted},
		{"Unauthenticated", apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthUnauthenticated)), http.StatusUnauthorized, i18n.CodeAccountAuthUnauthenticated},
		{"PanelHostReserved", apperrors.Forbidden(i18n.Msg(i18n.CodePlatformRoutingPanelHostReserved)), http.StatusForbidden, i18n.CodePlatformRoutingPanelHostReserved},
		{"CSRFFailed", apperrors.BadRequest(i18n.Msg(i18n.CodeTransportSecurityCSRFFailed)), http.StatusBadRequest, i18n.CodeTransportSecurityCSRFFailed},
		{"Internal", apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(assert.AnError)), http.StatusInternalServerError, i18n.CodePlatformServerInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.status, tc.err.Status())
			assert.Equal(t, tc.code, tc.err.Code())
			assert.Equal(t, string(tc.code), tc.err.Error())
		})
	}
}

func TestAppError_MethodsExposeStableState(t *testing.T) {
	t.Parallel()

	err := apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithMetaMap(map[string]any{"key": "value"}))

	assert.Equal(t, http.StatusInternalServerError, err.Status())
	assert.Equal(t, i18n.CodePlatformServerInternalError, err.Code())
	assert.Equal(t, string(i18n.CodePlatformServerInternalError), err.Error())
	assert.Equal(t, i18n.Msg(i18n.CodePlatformServerInternalError), err.Message())
	assert.False(t, err.Logged())
	assert.Nil(t, err.Unwrap())
	assert.Equal(t, map[string]any{"key": "value"}, err.Meta())
}

func TestAppError_MetaReturnsCopy(t *testing.T) {
	t.Parallel()

	err := apperrors.BadRequest(i18n.Msg(i18n.CodeTransportSecurityCSRFFailed), apperrors.WithMetaMap(map[string]any{"field": "email"}))
	meta := err.Meta()
	meta["field"] = "mutated"

	assert.Equal(t, "email", err.Meta()["field"])
}

func TestAppError_IsMatchesByCode(t *testing.T) {
	t.Parallel()

	err := apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 1)))
	assert.True(t, stdErrors.Is(err, apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 1)))))
	assert.False(t, stdErrors.Is(err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidCredentials))))
}
