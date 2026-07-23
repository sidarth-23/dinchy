package requestmeta_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/sidarth-23/dinchy/internal/foundation/requestmeta"
)

func TestRequestInfo_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := requestmeta.WithRequestInfo(context.Background(), "1.2.3.4", "Mozilla/5.0")
	assert.Equal(t, "1.2.3.4", requestmeta.RemoteIPFrom(ctx))
	assert.Equal(t, "Mozilla/5.0", requestmeta.UserAgentFrom(ctx))
}
