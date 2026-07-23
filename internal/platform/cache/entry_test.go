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

func newTestClient(t *testing.T) (*goredis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func newEntry(client *goredis.Client, namespace string, ttl time.Duration) cache.Entry[sample] {
	return cache.NewEntry[sample](client, cache.NewKeyer("dinchy"), namespace, ttl)
}

func TestEntry_GetMissThenHit(t *testing.T) {
	t.Parallel()
	client, _ := newTestClient(t)
	entry := newEntry(client, "sample", time.Minute)
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
	client, mr := newTestClient(t)
	entry := newEntry(client, "sample", time.Hour)
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
	client, _ := newTestClient(t)
	entry := newEntry(client, "sample", time.Minute)
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
	client, _ := newTestClient(t)
	ctx := context.Background()
	first := newEntry(client, "one", time.Minute)
	second := newEntry(client, "two", time.Minute)

	require.NoError(t, first.Set(ctx, "id", sample{Name: "first"}))
	require.NoError(t, second.Set(ctx, "id", sample{Name: "second"}))

	got, hit, err := first.Get(ctx, "id")
	require.NoError(t, err)
	require.True(t, hit)
	assert.Equal(t, "first", got.Name)
}

func TestEntry_NilClientIsNoop(t *testing.T) {
	t.Parallel()
	entry := newEntry(nil, "sample", time.Minute)
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
	client, mr := newTestClient(t)
	entry := newEntry(client, "sample", time.Minute)
	ctx := context.Background()

	require.NoError(t, entry.Set(ctx, "a", sample{Name: "x"}))
	mr.Close() // backend outage: reads must return an error, not a false miss

	_, _, err := entry.Get(ctx, "a")
	assert.Error(t, err)
}
