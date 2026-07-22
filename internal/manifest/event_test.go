package manifest

import (
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
