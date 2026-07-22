package render_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/transport/render"
)

func TestErrorResponse_MarshalsAndReportsStatus(t *testing.T) {
	t.Parallel()

	resp := &render.ErrorResponse{
		Payload: render.ResponsePayload{
			Code:    string(i18n.CodeTransportRequestValidationFailed),
			Message: "Some fields need attention.",
			Meta:    map[string]any{"fields": []any{map[string]any{"message": "expected string"}}},
		},
	}

	assert.Equal(t, string(i18n.CodeTransportRequestValidationFailed), resp.Error())
	assert.Equal(t, 0, resp.GetStatus())

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"error"`)
	assert.Contains(t, string(raw), `"fields"`)
	assert.Contains(t, string(raw), `"expected string"`)
}

func TestResolve_LocalizesAndPreservesMeta(t *testing.T) {
	t.Parallel()

	renderer := render.NewRenderer(i18n.Default, false)
	resp := renderer.Resolve(language.English, apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 2))))

	assert.Equal(t, http.StatusConflict, resp.GetStatus())
	assert.Equal(t, string(i18n.CodeAccountAuthSetupCompleted), resp.Payload.Code)
	assert.Equal(t, "Setup has already been completed for users (2 users).", resp.Payload.Message)
	assert.Equal(t, map[string]any{"resource": "users", "count": 2}, resp.Payload.Meta)
	assert.Nil(t, resp.Payload.Debug)
}

func TestResponseFor_ValidationDetails(t *testing.T) {
	t.Parallel()

	renderer := render.NewRenderer(i18n.Default, false)
	detail := &huma.ErrorDetail{Message: "expected string", Location: "body.email", Value: "x"}
	resp := renderer.ResponseFor(language.English, http.StatusUnprocessableEntity, detail)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.GetStatus())
	assert.Equal(t, string(i18n.CodeTransportRequestValidationFailed), resp.Payload.Code)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"error"`)
	assert.Contains(t, string(raw), `"fields"`)
	assert.Contains(t, string(raw), `"body.email"`)
}

func TestResolve_ServerErrorRendersGenericButPreservesCode(t *testing.T) {
	t.Parallel()

	renderer := render.NewRenderer(i18n.Default, false)
	appErr := apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindUser), apperrors.WithCause(assert.AnError))
	resp := renderer.Resolve(language.English, appErr)

	assert.Equal(t, http.StatusInternalServerError, resp.GetStatus())
	assert.Equal(t, string(i18n.CodePlatformServerInternalError), resp.Payload.Code)
	assert.Equal(t, "An unexpected error occurred.", resp.Payload.Message)
	assert.Nil(t, resp.Payload.Meta)
	assert.Nil(t, resp.Payload.Debug)
	// The specific code stays on the error for logging and matching.
	assert.Equal(t, i18n.CodeDiagnosticsAuthLoginFindUser, appErr.Code())
}

func TestResolve_ExposeInternalAttachesDebug(t *testing.T) {
	t.Parallel()

	renderer := render.NewRenderer(i18n.Default, true)
	appErr := apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindUser), apperrors.WithCause(assert.AnError))
	resp := renderer.Resolve(language.English, appErr)

	assert.Equal(t, string(i18n.CodePlatformServerInternalError), resp.Payload.Code)
	require.NotNil(t, resp.Payload.Debug)
	assert.Equal(t, string(i18n.CodeDiagnosticsAuthLoginFindUser), resp.Payload.Debug.Code)
	assert.Equal(t, assert.AnError.Error(), resp.Payload.Debug.Cause)
}

func TestResolve_ExposeInternalOnClientErrorAttachesDebug(t *testing.T) {
	t.Parallel()

	renderer := render.NewRenderer(i18n.Default, true)
	appErr := apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthOrganizationNotFound))
	resp := renderer.Resolve(language.English, appErr)

	assert.Equal(t, http.StatusBadRequest, resp.GetStatus())
	assert.Equal(t, string(i18n.CodeAccountAuthOrganizationNotFound), resp.Payload.Code)
	require.NotNil(t, resp.Payload.Debug)
	assert.Equal(t, string(i18n.CodeAccountAuthOrganizationNotFound), resp.Payload.Debug.Code)
}
