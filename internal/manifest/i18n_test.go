package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestMergeI18nCatalogsDeepMergesByName(t *testing.T) {
	t.Parallel()

	first := I18nCatalog{Modules: []I18nModule{
		{Name: "account", Modules: []I18nModule{
			{Name: "auth", Messages: []I18nMessage{
				{Name: "forbidden", Translations: map[string]string{"en": "no"}},
			}},
		}},
	}}
	second := I18nCatalog{Modules: []I18nModule{
		{Name: "account", Modules: []I18nModule{
			{Name: "auth", Messages: []I18nMessage{
				{Name: "unauthenticated", Translations: map[string]string{"en": "who"}},
			}},
			{Name: "access", Messages: []I18nMessage{
				{Name: "admin", Translations: map[string]string{"en": "Administrator"}},
			}},
		}},
	}}

	merged, err := MergeI18nCatalogs(first, second)
	require.NoError(t, err)
	require.NoError(t, ValidateI18nCatalog(merged))

	require.Len(t, merged.Modules, 1)
	account := merged.Modules[0]
	require.Equal(t, "account", account.Name)
	require.Len(t, account.Modules, 2)
	require.Equal(t, "auth", account.Modules[0].Name)
	require.Len(t, account.Modules[0].Messages, 2)
	require.Equal(t, "access", account.Modules[1].Name)
}

func TestMergeI18nCatalogsRejectsConflictingDescriptions(t *testing.T) {
	t.Parallel()

	first := I18nCatalog{Modules: []I18nModule{{Name: "account", Description: "one", Messages: []I18nMessage{
		{Name: "forbidden", Translations: map[string]string{"en": "no"}},
	}}}}
	second := I18nCatalog{Modules: []I18nModule{{Name: "account", Description: "two", Messages: []I18nMessage{
		{Name: "unauthenticated", Translations: map[string]string{"en": "who"}},
	}}}}

	_, err := MergeI18nCatalogs(first, second)
	require.ErrorContains(t, err, "conflicting descriptions")
}

func TestLoadI18nCatalogDirMergesSortedFragments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "account.json"), []byte(
		`{"modules":[{"name":"account","modules":[{"name":"auth","messages":[{"name":"forbidden","translations":{"en":"no"}}]}]}]}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "platform.json"), []byte(
		`{"modules":[{"name":"platform","modules":[{"name":"config","messages":[{"name":"load_failed","translations":{"en":"boom"}}]}]}]}`), 0o644))

	catalog, err := LoadI18nCatalogDir(dir)
	require.NoError(t, err)
	require.NoError(t, ValidateI18nCatalog(catalog))
	require.Len(t, catalog.Modules, 2)
	require.Equal(t, "account", catalog.Modules[0].Name)
	require.Equal(t, "platform", catalog.Modules[1].Name)
}

func TestLoadI18nCatalogDirRejectsEmptyDir(t *testing.T) {
	t.Parallel()

	_, err := LoadI18nCatalogDir(t.TempDir())
	require.ErrorContains(t, err, "no i18n fragments")
}

func TestLoadI18nCatalogDirWrapsDecodeErrorWithFilename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{"modules":[],"extra":true}`), 0o644))

	_, err := LoadI18nCatalogDir(dir)
	require.ErrorContains(t, err, "broken.json")
}
