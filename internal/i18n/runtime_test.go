package i18n_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

func TestCatalogResolve_InterpolatesMetadata(t *testing.T) {
	t.Parallel()
	got := i18n.Default.Resolve(language.English, i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", 1)))
	assert.Equal(t, "Setup has already been completed for users (1 users).", got)
}

func TestCatalogResolve_IsExactOnly(t *testing.T) {
	t.Parallel()
	got := i18n.Default.Resolve(language.English, i18n.Msg(i18n.CodeAuthInvalidCredentials))
	assert.Equal(t, "Invalid email or password.", got)
}
