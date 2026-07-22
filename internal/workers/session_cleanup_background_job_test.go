package workers

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

func TestSessionCleanupWorker_Execute_UsesRetentionWindow(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)

	store.EXPECT().
		DeleteEndedSessionsOlderThan(gomock.Any(), sqlcgen.DeleteEndedSessionsOlderThanParams{
			ExpiresAt: sqltype.Timestamptz(fixedTime.Add(-sessionCleanupRetentionDuration)),
			UpdatedAt: sqltype.Timestamptz(fixedTime),
		}).
		Return(pgconn.NewCommandTag("DELETE 7"), nil)

	worker := NewSessionCleanupWorker(store, clock.Fixed(fixedTime))
	outcome, err := worker.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(7), outcome.DeletedCount)
}

func TestSessionCleanupWorker_Execute_PropagatesError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	sentinel := stderrors.New("delete failed")

	store.EXPECT().
		DeleteEndedSessionsOlderThan(gomock.Any(), sqlcgen.DeleteEndedSessionsOlderThanParams{
			ExpiresAt: sqltype.Timestamptz(fixedTime.Add(-sessionCleanupRetentionDuration)),
			UpdatedAt: sqltype.Timestamptz(fixedTime),
		}).
		Return(pgconn.NewCommandTag("DELETE 0"), sentinel)

	worker := NewSessionCleanupWorker(store, clock.Fixed(fixedTime))
	outcome, err := worker.Execute(context.Background())
	require.ErrorIs(t, err, sentinel, "Execute returns the store error unwrapped; the runtime annotates it")
	assert.Zero(t, outcome.DeletedCount)
}

func TestSessionCleanupWorker_Contract(t *testing.T) {
	t.Parallel()
	worker := NewSessionCleanupWorker(nil, clock.Fixed(fixedTime))

	// The runtime keys scheduling and failure reporting on these values.
	assert.Equal(t, "session_cleanup", worker.TaskName())
	assert.Equal(t, int64(300), worker.IntervalSeconds())
	assert.Equal(t, 15*time.Second, worker.LeaseDuration())
	assert.Equal(t, 5*time.Minute, worker.RetryDelay())
	assert.Equal(t, "task.session_cleanup_failed", worker.FailureErrorCode())
	assert.Equal(t, i18n.CodeDiagnosticsWorkersSessionCleanup, worker.ExecutionCode())
}
