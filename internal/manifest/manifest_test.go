package manifest

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeEventCatalogRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	_, err := DecodeEventCatalog([]byte(`{"modules":[],"extra":true}`))
	require.Error(t, err)
}

func TestValidateEventCatalogRejectsInvalidTypedKeyType(t *testing.T) {
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
										MetadataKeys: []TypedKey{
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

func TestValidateErrorCatalogRejectsTypedValueMismatch(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"modules": [
			{
				"name": "metadata",
				"modules": [
					{
						"name": "deleted_count",
						"type": "int",
						"values": ["nope"]
					}
				]
			}
		]
	}`)

	catalog, err := DecodeErrorCatalog(raw)
	require.NoError(t, err)
	require.Error(t, ValidateErrorCatalog(catalog))
}

func TestDecodeErrorCatalogReadsTypedValues(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"modules": [
			{
				"name": "metadata",
				"modules": [
					{
						"name": "enabled",
						"type": "bool",
						"values": [true, false]
					}
				]
			}
		]
	}`)

	catalog, err := DecodeErrorCatalog(raw)
	require.NoError(t, err)
	require.Len(t, catalog.Modules, 1)

	encoded, err := json.Marshal(catalog)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"enabled"`)
}
