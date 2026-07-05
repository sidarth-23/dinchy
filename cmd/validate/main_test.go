package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	manifest "github.com/sidarth-23/dinchy/internal/manifest"
)

func TestRunValidateErrorAcceptsValidCatalog(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "catalog.json")
	raw, err := json.Marshal(manifest.ErrorCatalog{
		Modules: []manifest.ErrorModule{
			{
				Name: "metadata",
				Modules: []manifest.ErrorNode{
					{
						Name: "stage",
						Type: "string",
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))
	require.NoError(t, runValidateError([]string{"-input", path}))
}

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
										MetadataKeys: []manifest.TypedKey{
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
