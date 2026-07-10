package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/platform/cache"
)

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func newTestCache(t *testing.T, enabled bool) (cache.Cache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return cache.NewRedis(client, cache.NewKeyer("dinchy"), enabled), mr
}

func TestEntry_GetMissThenHit(t *testing.T) {
	t.Parallel()
	c, _ := newTestCache(t, true)
	entry := cache.NewEntry[sample](c, "sample", time.Minute)
	ctx := context.Background()

	_, hit, err := entry.Get(ctx, "a")
	require.NoError(t, err)
	assert.False(t, hit)

	require.NoError(t, entry.Set(ctx, "a", sample{Name: "x", Count: 3}))

	got, hit, err := entry.Get(ctx, "a")
	require.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, sample{Name: "x", Count: 3}, got)
}

func TestEntry_SetWithTTLExpires(t *testing.T) {
	t.Parallel()
	c, mr := newTestCache(t, true)
	entry := cache.NewEntry[sample](c, "sample", time.Hour)
	ctx := context.Background()

	require.NoError(t, entry.SetWithTTL(ctx, "a", sample{Name: "x"}, 30*time.Second))
	_, hit, err := entry.Get(ctx, "a")
	require.NoError(t, err)
	assert.True(t, hit)

	mr.FastForward(31 * time.Second)
	_, hit, err = entry.Get(ctx, "a")
	require.NoError(t, err)
	assert.False(t, hit)
}

func TestEntry_DeleteRemovesKeys(t *testing.T) {
	t.Parallel()
	c, _ := newTestCache(t, true)
	entry := cache.NewEntry[sample](c, "sample", time.Minute)
	ctx := context.Background()

	require.NoError(t, entry.Set(ctx, "a", sample{Name: "a"}))
	require.NoError(t, entry.Set(ctx, "b", sample{Name: "b"}))
	require.NoError(t, entry.Delete(ctx, "a", "b"))

	_, hit, err := entry.Get(ctx, "a")
	require.NoError(t, err)
	assert.False(t, hit)
	_, hit, err = entry.Get(ctx, "b")
	require.NoError(t, err)
	assert.False(t, hit)
}

func TestEntry_NamespacesDoNotCollide(t *testing.T) {
	t.Parallel()
	c, _ := newTestCache(t, true)
	ctx := context.Background()
	first := cache.NewEntry[sample](c, "one", time.Minute)
	second := cache.NewEntry[sample](c, "two", time.Minute)

	require.NoError(t, first.Set(ctx, "id", sample{Name: "first"}))
	require.NoError(t, second.Set(ctx, "id", sample{Name: "second"}))

	got, hit, err := first.Get(ctx, "id")
	require.NoError(t, err)
	require.True(t, hit)
	assert.Equal(t, "first", got.Name)
}

func TestEntry_DisabledCacheIsNoop(t *testing.T) {
	t.Parallel()
	c, _ := newTestCache(t, false)
	entry := cache.NewEntry[sample](c, "sample", time.Minute)
	ctx := context.Background()

	require.False(t, entry.Enabled())
	require.NoError(t, entry.Set(ctx, "a", sample{Name: "x"}))
	_, hit, err := entry.Get(ctx, "a")
	require.NoError(t, err)
	assert.False(t, hit)
}

func TestEntry_NilCacheIsNoop(t *testing.T) {
	t.Parallel()
	entry := cache.NewEntry[sample](nil, "sample", time.Minute)
	ctx := context.Background()

	require.False(t, entry.Enabled())
	require.NoError(t, entry.Set(ctx, "a", sample{Name: "x"}))
	_, hit, err := entry.Get(ctx, "a")
	require.NoError(t, err)
	assert.False(t, hit)
	require.NoError(t, entry.Delete(ctx, "a"))
}

func TestEntry_ReadErrorSurfaces(t *testing.T) {
	t.Parallel()
	c, mr := newTestCache(t, true)
	entry := cache.NewEntry[sample](c, "sample", time.Minute)
	ctx := context.Background()

	require.NoError(t, entry.Set(ctx, "a", sample{Name: "x"}))
	mr.Close() // backend outage: reads must return an error, not a false miss

	_, _, err := entry.Get(ctx, "a")
	assert.Error(t, err)
}
