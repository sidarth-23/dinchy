package requestcontext_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/foundation/requestcontext"
)

func TestRequestInfo_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := requestcontext.WithRequestInfo(context.Background(), "1.2.3.4", "Mozilla/5.0")
	assert.Equal(t, "1.2.3.4", requestcontext.RemoteIPFrom(ctx))
	assert.Equal(t, "Mozilla/5.0", requestcontext.UserAgentFrom(ctx))
}
