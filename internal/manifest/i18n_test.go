package manifest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeI18nCatalogRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodeI18nCatalog([]byte(`{"modules":[],"extra":true}`))
	require.Error(t, err)
}

func TestValidateI18nCatalogRejectsInvalidNestedMessage(t *testing.T) {
	t.Parallel()

	err := ValidateI18nCatalog(I18nCatalog{
		Modules: []I18nModule{
			{
				Name: "auth",
				Messages: []I18nMessage{
					{
						Name: "setup_completed",
						Params: []I18nParam{
							{Name: "resource", Type: "string"},
							{Name: "resource", Type: "int"},
						},
						Translations: map[string]string{"en": "ok"},
					},
				},
			},
		},
	})
	require.Error(t, err)
}

func TestValidateI18nCatalogRejectsMissingEnglishTranslation(t *testing.T) {
	t.Parallel()

	err := ValidateI18nCatalog(I18nCatalog{
		Modules: []I18nModule{
			{
				Name: "auth",
				Messages: []I18nMessage{
					{
						Name:         "setup_completed",
						Translations: map[string]string{"fr": "bonjour"},
					},
				},
			},
		},
	})
	require.Error(t, err)
}

func TestDecodeI18nCatalogReadsNestedMessages(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"modules": [
			{
				"name": "auth",
				"modules": [
					{
						"name": "oidc",
						"messages": [
							{
								"name": "provider_not_found",
								"params": [{"name": "provider", "type": "string"}],
								"translations": {"en": "Missing"}
							}
						]
					}
				]
			}
		]
	}`)

	catalog, err := DecodeI18nCatalog(raw)
	require.NoError(t, err)
	require.Len(t, catalog.Modules, 1)
	require.Equal(t, "auth.oidc.provider_not_found", I18nCodeFor([]string{"auth", "oidc"}, "provider_not_found"))
	require.Equal(t, "AuthOIDCProviderNotFound", I18nConstantName([]string{"auth", "oidc"}, "provider_not_found"))

	encoded, err := json.Marshal(catalog)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"provider_not_found"`)
}
