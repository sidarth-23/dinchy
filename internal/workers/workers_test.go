package workers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/config"
)

func TestNew_BuildsScheduler(t *testing.T) {
	t.Parallel()
	sched, err := New(nil, config.DefaultWorker())
	require.NoError(t, err)
	require.NotNil(t, sched)
	t.Cleanup(func() { _ = sched.Shutdown() })
}

func TestNew_RunsJobAndDrainsOnShutdown(t *testing.T) {
	t.Parallel()
	sched, err := New(nil, config.DefaultWorker())
	require.NoError(t, err)

	var mu sync.Mutex
	runs := 0
	done := make(chan struct{})
	_, err = sched.NewJob(
		gocron.OneTimeJob(gocron.OneTimeJobStartImmediately()),
		gocron.NewTask(func(_ context.Context) error {
			mu.Lock()
			runs++
			mu.Unlock()
			close(done)
			return nil
		}),
		gocron.WithName("test.oneoff"),
	)
	require.NoError(t, err)
	sched.Start()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run within timeout")
	}
	require.NoError(t, sched.Shutdown())

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, runs)
}
