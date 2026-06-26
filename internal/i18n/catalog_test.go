package i18n_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

func TestCatalogResolve_InterpolatesMetadata(t *testing.T) {
	t.Parallel()

	catalog := i18n.New(language.English)
	catalog.Register(language.English, i18n.Messages{
		AuthInvalidCredentials:  "Invalid email or password.",
		AuthSetupCompleted:      "Setup has already been completed for {{.resource}} ({{.count}} users).",
		AuthUnauthenticated:     "Authentication required.",
		SecurityHTTPSRequired:   "This endpoint requires a secure (HTTPS) connection.",
		SecurityCSRFFailed:      "Missing or invalid CSRF token.",
		RequestValidationFailed: "Some fields need attention.",
		ConfigLoadFailed:        "Failed to load configuration.",
		ConfigValidationFailed:  "Configuration is invalid.",
		ServerInternalError:     "An unexpected error occurred.",
	})

	got := catalog.Resolve(language.English, "auth.setup_completed", map[string]any{
		"resource": "users",
		"count":    3,
	})

	assert.Equal(t, "Setup has already been completed for users (3 users).", got)
}

func TestCatalogResolve_FallsBackToCode(t *testing.T) {
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

	assert.Equal(t, "missing.code", catalog.Resolve(language.English, "missing.code", nil))
}
