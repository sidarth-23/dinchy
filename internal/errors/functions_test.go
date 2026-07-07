package errors_test

import (
	"encoding/json"
	stdErrors "errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/language"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

func TestConstructors_StatusAndCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    *apperrors.AppError
		status int
		code   i18n.Code
	}{
		{"InvalidCredentials", apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials)), http.StatusUnauthorized, i18n.CodeAuthInvalidCredentials},
		{"SetupCompleted", apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 3))), http.StatusConflict, i18n.CodeAuthSetupCompleted},
		{"Unauthenticated", apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated)), http.StatusUnauthorized, i18n.CodeAuthUnauthenticated},
		{"HTTPSRequired", apperrors.Forbidden(i18n.Msg(i18n.CodeSecurityHTTPSRequired)), http.StatusForbidden, i18n.CodeSecurityHTTPSRequired},
		{"CSRFFailed", apperrors.BadRequest(i18n.Msg(i18n.CodeSecurityCSRFFailed)), http.StatusBadRequest, i18n.CodeSecurityCSRFFailed},
		{"Internal", apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(assert.AnError)), http.StatusInternalServerError, i18n.CodeServerInternalError},
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

func TestAnnotatePreservesCodeAndAddsMeta(t *testing.T) {
	t.Parallel()

	base := apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 2)), apperrors.WithStage(apperrors.StageSetup))
	base.MarkLogged()
	err := apperrors.Annotate(base, apperrors.WithStage(apperrors.StageSetup))

	var got *apperrors.AppError
	require.ErrorAs(t, err, &got)
	assert.Equal(t, i18n.CodeAuthSetupCompleted, got.Code())
	assert.Equal(t, "users", got.Meta()["resource"])
	assert.Equal(t, 2, got.Meta()["count"])
	assert.Equal(t, "setup", got.Meta()["stage"])
	assert.True(t, got.Logged())
	assert.True(t, stdErrors.Is(err, apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 2)))))
}

func TestResolve_LocalizesAndPreservesMeta(t *testing.T) {
	t.Parallel()

	catalog := i18n.New(i18n.CatalogData)
	resp := apperrors.Resolve(language.English, catalog, apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 2))))

	assert.Equal(t, http.StatusConflict, resp.GetStatus())
	assert.Equal(t, string(i18n.CodeAuthSetupCompleted), resp.Payload.Code)
	assert.Equal(t, "Setup has already been completed for users (2 users).", resp.Payload.Message)
	assert.Equal(t, map[string]any{"resource": "users", "count": 2}, resp.Payload.Meta)
}

func TestResponseFor_ValidationDetails(t *testing.T) {
	t.Parallel()

	catalog := i18n.Default
	detail := &huma.ErrorDetail{Message: "expected string", Location: "body.email", Value: "x"}
	resp := apperrors.ResponseFor(language.English, catalog, http.StatusUnprocessableEntity, detail)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.GetStatus())
	assert.Equal(t, string(i18n.CodeRequestValidationFailed), resp.Payload.Code)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"error"`)
	assert.Contains(t, string(raw), `"fields"`)
	assert.Contains(t, string(raw), `"body.email"`)
}
