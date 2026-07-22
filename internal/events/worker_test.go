package events

import (
	"context"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-co-op/gocron/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSubscriber struct {
	name string
}

func (s testSubscriber) Name() string { return s.name }

func (testSubscriber) Logger(context.Context) *slog.Logger { return slog.Default() }

func (testSubscriber) Handle(context.Context, Record) error { return nil }

func TestRegisterWorkers_SchedulesJobPerSubscriber(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	svc, err := NewService(client, nil, Config{WorkerInterval: time.Second})
	require.NoError(t, err)
	svc.Register(testSubscriber{name: "audit"})
	svc.Register(testSubscriber{name: "billing"})

	sched, err := gocron.NewScheduler()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sched.Shutdown() })

	require.NoError(t, RegisterWorkers(sched, svc))

	names := make([]string, 0)
	for _, job := range sched.Jobs() {
		names = append(names, job.Name())
	}
	sort.Strings(names)
	assert.Equal(t, []string{"eventbus.audit", "eventbus.billing"}, names)
}

func TestRegisterWorkers_NilServiceIsNoop(t *testing.T) {
	sched, err := gocron.NewScheduler()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sched.Shutdown() })

	require.NoError(t, RegisterWorkers(sched, nil))
	assert.Empty(t, sched.Jobs())
}
