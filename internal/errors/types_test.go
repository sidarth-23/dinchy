package errors_test

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

func TestAppError_MethodsExposeStableState(t *testing.T) {
	t.Parallel()

	err := apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithMetaMap(map[string]any{"key": "value"}))

	assert.Equal(t, http.StatusInternalServerError, err.Status())
	assert.Equal(t, i18n.CodeServerInternalError, err.Code())
	assert.Equal(t, string(i18n.CodeServerInternalError), err.Error())
	assert.Equal(t, i18n.Msg(i18n.CodeServerInternalError), err.Message())
	assert.False(t, err.Logged())
	assert.Nil(t, err.Unwrap())
	assert.Equal(t, map[string]any{"key": "value"}, err.Meta())
}

func TestAppError_MetaReturnsCopy(t *testing.T) {
	t.Parallel()

	err := apperrors.BadRequest(i18n.Msg(i18n.CodeSecurityCSRFFailed), apperrors.WithMetaMap(map[string]any{"field": "email"}))
	meta := err.Meta()
	meta["field"] = "mutated"

	assert.Equal(t, "email", err.Meta()["field"])
}

func TestErrorResponse_MarshalsAndReportsStatus(t *testing.T) {
	t.Parallel()

	resp := &apperrors.ErrorResponse{
		Payload: apperrors.ResponsePayload{
			Code:    string(i18n.CodeRequestValidationFailed),
			Message: "Some fields need attention.",
			Meta:    map[string]any{"fields": []any{map[string]any{"message": "expected string"}}},
		},
	}

	assert.Equal(t, string(i18n.CodeRequestValidationFailed), resp.Error())
	assert.Equal(t, 0, resp.GetStatus())

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"error"`)
	assert.Contains(t, string(raw), `"fields"`)
	assert.Contains(t, string(raw), `"expected string"`)
}

func TestAppError_IsMatchesByCode(t *testing.T) {
	t.Parallel()

	err := apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 1)))
	assert.True(t, stdErrors.Is(err, apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 1)))))
	assert.False(t, stdErrors.Is(err, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))))
}
