package render

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
