package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunMatchesCheckedInOutput(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "generated.go")
	input := filepath.Join("..", "..", "internal", "errors", "meta.json")
	wantPath := filepath.Join("..", "..", "internal", "errors", "generated.go")

	require.NoError(t, run(input, out))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	want, err := os.ReadFile(wantPath)
	require.NoError(t, err)

	require.Equal(t, string(want), string(got))
}

func TestValidateRejectsDuplicateMetaKeys(t *testing.T) {
	t.Parallel()

	err := validate(manifest{
		Groups: []group{
			{Name: "A", MetaKey: "same"},
			{Name: "B", MetaKey: "same"},
		},
	})
	require.Error(t, err)
}

