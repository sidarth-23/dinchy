package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeEventCatalogRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodeEventCatalog([]byte(`{"modules":[],"extra":true}`))
	require.Error(t, err)
}

func TestValidateEventCatalogRejectsInvalidTypedFieldType(t *testing.T) {
	t.Parallel()

	catalog := EventCatalog{
		Modules: []EventModule{
			{
				ID: "auth",
				Modules: []EventModule{
					{
						ID: "security",
						Modules: []EventModule{
							{
								ID: "auth",
								Events: []EventDefinition{
									{
										ID:      "login",
										Action:  "login",
										Outcome: "succeeded",
										MetadataKeys: []Field{
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
	}

	err := ValidateEventCatalog(catalog)
	require.Error(t, err)
}

func TestMergeEventCatalogsDeepMergesByID(t *testing.T) {
	t.Parallel()

	first := EventCatalog{Modules: []EventModule{
		{ID: "auth", Modules: []EventModule{
			{ID: "security", Events: []EventDefinition{
				{ID: "login_succeeded", Action: "login", Outcome: "succeeded"},
			}},
		}},
	}}
	second := EventCatalog{Modules: []EventModule{
		{ID: "auth", Modules: []EventModule{
			{ID: "security", Events: []EventDefinition{
				{ID: "logout_succeeded", Action: "logout", Outcome: "succeeded"},
			}},
			{ID: "billing", Events: []EventDefinition{
				{ID: "invoice_paid", Action: "pay_invoice", Outcome: "succeeded"},
			}},
		}},
	}}

	merged, err := MergeEventCatalogs(first, second)
	require.NoError(t, err)
	require.NoError(t, ValidateEventCatalog(merged))

	require.Len(t, merged.Modules, 1)
	auth := merged.Modules[0]
	require.Equal(t, "auth", auth.ID)
	require.Len(t, auth.Modules, 2)
	require.Equal(t, "security", auth.Modules[0].ID)
	require.Len(t, auth.Modules[0].Events, 2)
	require.Equal(t, "billing", auth.Modules[1].ID)
}

func TestMergeEventCatalogsRejectsConflictingDescriptions(t *testing.T) {
	t.Parallel()

	first := EventCatalog{Modules: []EventModule{{ID: "auth", Description: "one", Events: []EventDefinition{
		{ID: "login_succeeded", Action: "login", Outcome: "succeeded"},
	}}}}
	second := EventCatalog{Modules: []EventModule{{ID: "auth", Description: "two", Events: []EventDefinition{
		{ID: "logout_succeeded", Action: "logout", Outcome: "succeeded"},
	}}}}

	_, err := MergeEventCatalogs(first, second)
	require.ErrorContains(t, err, "conflicting descriptions")
}

func TestLoadEventCatalogDirMergesSortedFragments(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "auth.json"), []byte(
		`{"modules":[{"id":"auth","modules":[{"id":"security","events":[{"id":"login_succeeded","action":"login","outcome":"succeeded"}]}]}]}`,
	), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "billing.json"), []byte(
		`{"modules":[{"id":"billing","events":[{"id":"invoice_paid","action":"pay_invoice","outcome":"succeeded"}]}]}`,
	), 0o644))

	catalog, err := LoadEventCatalogDir(dir)
	require.NoError(t, err)
	require.NoError(t, ValidateEventCatalog(catalog))
	require.Len(t, catalog.Modules, 2)
	require.Equal(t, "auth", catalog.Modules[0].ID)
	require.Equal(t, "billing", catalog.Modules[1].ID)
}

func TestLoadEventCatalogDirRejectsEmptyDir(t *testing.T) {
	t.Parallel()

	_, err := LoadEventCatalogDir(t.TempDir())
	require.ErrorContains(t, err, "no event fragments")
}
