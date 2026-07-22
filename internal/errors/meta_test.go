package errors

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
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

func TestValidationDetailsCollectsErrorDetailerAndFallback(t *testing.T) {
	t.Parallel()

	details := validationDetails(testErrorDetailer{
		detail: &huma.ErrorDetail{Message: "expected string", Location: "body.email", Value: "x"},
	}, assert.AnError)

	require.Len(t, details, 2)
	assert.Equal(t, "expected string", details[0]["message"])
	assert.Equal(t, "body.email", details[0]["location"])
	assert.Equal(t, "x", details[0]["value"])
	assert.Equal(t, assert.AnError.Error(), details[1]["message"])
}

type testErrorDetailer struct {
	detail *huma.ErrorDetail
}

func (t testErrorDetailer) Error() string {
	if t.detail == nil {
		return ""
	}
	return t.detail.Message
}

func (t testErrorDetailer) ErrorDetail() *huma.ErrorDetail {
	return t.detail
}
