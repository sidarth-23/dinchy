package errors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/i18n"
)

func TestWithMetaMapCopiesInput(t *testing.T) {
	t.Parallel()

	err := Internal(i18n.Msg(i18n.CodePlatformServerInternalError), WithMetaMap(map[string]any{"key": "value"}))
	meta := err.Meta()
	assert.Equal(t, "value", meta["key"])

	source := map[string]any{"key": "value"}
	WithMetaMap(source)(err)
	source["key"] = "changed"
	assert.Equal(t, "value", err.Meta()["key"])
}

func TestMergeMetaCombinesBothMaps(t *testing.T) {
	t.Parallel()

	got := mergeMeta(map[string]any{"left": 1}, map[string]any{"right": 2})
	require.Equal(t, map[string]any{"left": 1, "right": 2}, got)
	assert.Nil(t, mergeMeta(nil, nil))
}
