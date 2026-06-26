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
		code   string
	}{
		{"InvalidCredentials", apperrors.InvalidCredentials(), http.StatusUnauthorized, apperrors.CodeAuthInvalidCredentials},
		{"SetupCompleted", apperrors.SetupCompleted(), http.StatusConflict, apperrors.CodeAuthSetupCompleted},
		{"Unauthenticated", apperrors.Unauthenticated(), http.StatusUnauthorized, apperrors.CodeAuthUnauthenticated},
		{"HTTPSRequired", apperrors.HTTPSRequired(), http.StatusForbidden, apperrors.CodeSecurityHTTPSRequired},
		{"CSRFFailed", apperrors.CSRFFailed(), http.StatusBadRequest, apperrors.CodeSecurityCSRFFailed},
		{"Internal", apperrors.Internal(assert.AnError), http.StatusInternalServerError, apperrors.CodeServerInternalError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.status, tc.err.Status())
			assert.Equal(t, tc.code, tc.err.Code())
			assert.Equal(t, tc.code, tc.err.Error())
		})
	}
}

func TestAppError_IsMatchesByCode(t *testing.T) {
	t.Parallel()

	err := apperrors.SetupCompleted()
	assert.True(t, stdErrors.Is(err, apperrors.SetupCompleted()))
	assert.False(t, stdErrors.Is(err, apperrors.InvalidCredentials()))
}

func TestResolve_LocalizesAndPreservesMeta(t *testing.T) {
	t.Parallel()

	catalog := i18n.New(language.English)
	catalog.Register(language.English, i18n.Messages{
		AuthInvalidCredentials:  "Invalid email or password.",
		AuthSetupCompleted:      "Setup has already been completed.",
		AuthUnauthenticated:     "Authentication required.",
		SecurityHTTPSRequired:   "This endpoint requires a secure (HTTPS) connection.",
		SecurityCSRFFailed:      "Missing or invalid CSRF token.",
		RequestValidationFailed: "Some fields need attention.",
		ConfigLoadFailed:        "Failed to load configuration.",
		ConfigValidationFailed:  "Configuration is invalid.",
		ServerInternalError:     "An unexpected error occurred.",
	})

	resp := apperrors.Resolve(language.English, catalog, apperrors.SetupCompleted(
		apperrors.WithMeta("resource", "users"),
		apperrors.WithMeta("count", 2),
	))

	assert.Equal(t, http.StatusConflict, resp.GetStatus())
	assert.Equal(t, "auth.setup_completed", resp.Payload.Code)
	assert.Equal(t, "Setup has already been completed.", resp.Payload.Message)
	assert.Equal(t, map[string]any{"resource": "users", "count": 2}, resp.Payload.Meta)
}

func TestResponseFor_ValidationDetails(t *testing.T) {
	t.Parallel()

	catalog := i18n.Default
	detail := &huma.ErrorDetail{Message: "expected string", Location: "body.email", Value: "x"}
	resp := apperrors.ResponseFor(language.English, catalog, http.StatusUnprocessableEntity, "validation failed", detail)

	assert.Equal(t, http.StatusUnprocessableEntity, resp.GetStatus())
	assert.Equal(t, apperrors.CodeRequestValidationFailed, resp.Payload.Code)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"error"`)
	assert.Contains(t, string(raw), `"fields"`)
	assert.Contains(t, string(raw), `"body.email"`)
}
