package cache_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
)

func TestOpenRedis_ConnectsToRedis(t *testing.T) {
	t.Parallel()

	server := miniredis.RunT(t)
	client, err := cache.OpenRedis(context.Background(), config.RedisConfig{Addr: server.Addr()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.NotNil(t, client)
}
