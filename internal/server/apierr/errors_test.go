package apierr_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/auth"
	"github.com/sidarth-23/dinchy/internal/server/apierr"
	"github.com/sidarth-23/dinchy/internal/server/support"
)

func TestErrorConstructors_StatusCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		err    *apierr.DinchyError
		status int
		code   string
	}{
		{"InvalidCredentials", apierr.ErrInvalidCredentials(), http.StatusUnauthorized, "auth.invalid_credentials"},
		{"SetupCompleted", apierr.ErrSetupCompleted(), http.StatusConflict, "auth.setup_completed"},
		{"Unauthenticated", apierr.ErrUnauthenticated(), http.StatusUnauthorized, "auth.unauthenticated"},
		{"HTTPSRequired", apierr.ErrHTTPSRequired(), http.StatusForbidden, "security.https_required"},
		{"CSRFFailed", apierr.ErrCSRFFailed(), http.StatusBadRequest, "security.csrf_failed"},
		{"Internal", apierr.ErrInternal(), http.StatusInternalServerError, "server.internal_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.status, tc.err.GetStatus())
			assert.Equal(t, tc.code, tc.err.Error())
		})
	}
}

func TestLocalized_EnglishMessage(t *testing.T) {
	t.Parallel()
	ctx := support.WithLang(context.Background(), language.English)
	loc := apierr.Localized(ctx, apierr.ErrInvalidCredentials())
	assert.Equal(t, "Invalid email or password.", loc.Message)
	assert.Equal(t, "auth.invalid_credentials", loc.Code)
	assert.Equal(t, http.StatusUnauthorized, loc.GetStatus())
}

func TestLocalized_UnknownLangFallsBackToEnglish(t *testing.T) {
	t.Parallel()
	ctx := support.WithLang(context.Background(), language.MustParse("sw")) // Swahili — not registered
	loc := apierr.Localized(ctx, apierr.ErrInternal())
	assert.Equal(t, "An unexpected error occurred.", loc.Message)
}

func TestLocalized_NoLangInContextDefaultsToEnglish(t *testing.T) {
	t.Parallel()
	loc := apierr.Localized(context.Background(), apierr.ErrUnauthenticated())
	assert.Equal(t, "Authentication required.", loc.Message)
}

func TestMapServiceError_InvalidCredentials(t *testing.T) {
	t.Parallel()
	loc := apierr.MapServiceError(context.Background(), auth.ErrInvalidCredentials)
	assert.Equal(t, http.StatusUnauthorized, loc.GetStatus())
	assert.Equal(t, "auth.invalid_credentials", loc.Code)
}

func TestMapServiceError_SetupCompleted(t *testing.T) {
	t.Parallel()
	loc := apierr.MapServiceError(context.Background(), auth.ErrSetupCompleted)
	assert.Equal(t, http.StatusConflict, loc.GetStatus())
}

func TestMapServiceError_Unknown(t *testing.T) {
	t.Parallel()
	loc := apierr.MapServiceError(context.Background(), assert.AnError)
	assert.Equal(t, http.StatusInternalServerError, loc.GetStatus())
}
