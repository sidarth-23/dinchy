package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func TestRunValidateEventRejectsInvalidCatalog(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "catalog.json")
	raw, err := json.Marshal(manifest.EventCatalog{
		Modules: []manifest.EventModule{
			{
				ID: "auth",
				Modules: []manifest.EventModule{
					{
						ID: "security",
						Modules: []manifest.EventModule{
							{
								ID: "auth",
								Events: []manifest.EventDefinition{
									{
										ID:      "login",
										Action:  "login",
										Outcome: "succeeded",
										MetadataKeys: []manifest.Field{
											{Name: "email", Type: "uuid"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	require.Error(t, runValidateEvent([]string{"-input", path}))
}

func TestRunValidateI18nAcceptsValidCatalog(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "catalog.json")
	raw, err := json.Marshal(manifest.I18nCatalog{
		Modules: []manifest.I18nModule{
			{
				Name: "auth",
				Messages: []manifest.I18nMessage{
					{
						Name: "invalid_credentials",
						Translations: map[string]string{
							"en": "Invalid email or password.",
						},
					},
				},
				Modules: []manifest.I18nModule{
					{
						Name: "oidc",
						Messages: []manifest.I18nMessage{
							{
								Name: "provider_not_found",
								Params: []manifest.I18nParam{
									{Name: "provider", Type: "string"},
								},
								Translations: map[string]string{
									"en": "The selected OIDC provider is not available.",
								},
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	require.NoError(t, runValidateI18n([]string{"-input", path}))
}

func TestRunValidateI18nRejectsMissingEnglishTranslation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "catalog.json")
	raw, err := json.Marshal(manifest.I18nCatalog{
		Modules: []manifest.I18nModule{
			{
				Name: "auth",
				Messages: []manifest.I18nMessage{
					{
						Name: "invalid_credentials",
						Translations: map[string]string{
							"fr": "Identifiants invalides.",
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	require.Error(t, runValidateI18n([]string{"-input", path}))
}
