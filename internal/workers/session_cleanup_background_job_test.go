package workers

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/foundation/clock"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

type fakeSessionStore struct {
	params sqlcgen.DeleteEndedSessionsOlderThanParams
	tag    pgconn.CommandTag
	err    error
	calls  int
}

func (f *fakeSessionStore) DeleteEndedSessionsOlderThan(_ context.Context, arg sqlcgen.DeleteEndedSessionsOlderThanParams) (pgconn.CommandTag, error) {
	f.calls++
	f.params = arg
	return f.tag, f.err
}

func TestCleanupEndedSessions_UsesRetentionWindow(t *testing.T) {
	t.Parallel()
	store := &fakeSessionStore{tag: pgconn.NewCommandTag("DELETE 7")}

	require.NoError(t, cleanupEndedSessions(context.Background(), store, clock.Fixed(fixedTime)))
	assert.Equal(t, 1, store.calls)
	assert.Equal(t, sqltype.Timestamptz(fixedTime.Add(-sessionCleanupRetentionDuration)), store.params.ExpiresAt)
	assert.Equal(t, sqltype.Timestamptz(fixedTime), store.params.UpdatedAt)
}

func TestCleanupEndedSessions_WrapsStoreError(t *testing.T) {
	t.Parallel()
	sentinel := stderrors.New("delete failed")
	store := &fakeSessionStore{err: sentinel}

	err := cleanupEndedSessions(context.Background(), store, clock.Fixed(fixedTime))
	require.ErrorIs(t, err, sentinel)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, i18n.CodeDiagnosticsWorkersSessionCleanup, appErr.Code())
}

func TestRegisterSessionCleanup_SchedulesNamedJob(t *testing.T) {
	t.Parallel()
	sched, err := gocron.NewScheduler()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sched.Shutdown() })

	require.NoError(t, RegisterSessionCleanup(sched, &fakeSessionStore{tag: pgconn.NewCommandTag("DELETE 0")}, clock.Fixed(fixedTime)))

	jobs := sched.Jobs()
	require.Len(t, jobs, 1)
	assert.Equal(t, sessionCleanupJobName, jobs[0].Name())
}
