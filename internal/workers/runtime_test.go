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

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

var fixedTime = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time { return c.now }

// stubWorker is a configurable Worker so runtime tests can drive each branch
// without depending on a concrete worker implementation.
type stubWorker struct {
	name     string
	interval int64
	lease    time.Duration
	retry    time.Duration
	failCode string
	stage    apperrors.Stage
	outcome  WorkerOutcome
	execErr  error
	executed int
}

func (w *stubWorker) TaskName() string                { return w.name }
func (w *stubWorker) IntervalSeconds() int64          { return w.interval }
func (w *stubWorker) LeaseDuration() time.Duration    { return w.lease }
func (w *stubWorker) RetryDelay() time.Duration       { return w.retry }
func (w *stubWorker) FailureErrorCode() string        { return w.failCode }
func (w *stubWorker) ExecutionStage() apperrors.Stage { return w.stage }

func (w *stubWorker) Execute(context.Context) (WorkerOutcome, error) {
	w.executed++
	return w.outcome, w.execErr
}

func newStubWorker() *stubWorker {
	return &stubWorker{
		name:     "cleanup",
		interval: 300,
		lease:    15 * time.Second,
		retry:    5 * time.Minute,
		failCode: "task.cleanup_failed",
		stage:    apperrors.StageDeleteEndedSessions,
	}
}

func metaOf(t *testing.T, err error) map[string]any {
	t.Helper()
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	return appErr.Meta()
}

func TestRuntime_RegisterWorker_EnsuresTask(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	worker := newStubWorker()

	store.EXPECT().EnsureTask(gomock.Any(), sqlcgen.EnsureTaskParams{
		ID:                      taskIDForName(worker.name),
		TaskName:                worker.name,
		ScheduleIntervalSeconds: worker.interval,
		NextRunAt:               sqltype.Timestamptz(fixedTime),
		UpdatedAt:               sqltype.Timestamptz(fixedTime),
	}).Return(nil)

	runtime := NewRuntime(store, fakeClock{now: fixedTime}, nil, nil, worker)
	require.NoError(t, runtime.registerWorker(context.Background(), worker))
}

func TestRuntime_RegisterWorker_AnnotatesEnsureError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	worker := newStubWorker()

	store.EXPECT().EnsureTask(gomock.Any(), gomock.Any()).
		Return(apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError)))

	runtime := NewRuntime(store, fakeClock{now: fixedTime}, nil, nil, worker)
	err := runtime.registerWorker(context.Background(), worker)
	require.Error(t, err)

	meta := metaOf(t, err)
	assert.Equal(t, worker.name, meta["task"])
	assert.Equal(t, string(apperrors.StageEnsureTask), meta["stage"])
}

func TestRuntime_RunWorker_SkipsExecuteWhenNotClaimed(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	worker := newStubWorker()

	store.EXPECT().
		ClaimTask(gomock.Any(), sqlcgen.ClaimTaskParams{
			LeaseOwner:           sqltype.Text("local"),
			LeaseExpiresAt:       sqltype.Timestamptz(fixedTime.Add(worker.lease)),
			LastRunAt:            sqltype.Timestamptz(fixedTime),
			UpdatedAt:            sqltype.Timestamptz(fixedTime),
			TaskName:             worker.name,
			LeaseExpiresAtCutoff: sqltype.Timestamptz(fixedTime.Add(worker.lease)),
			NextRunAtCutoff:      sqltype.Timestamptz(fixedTime),
		}).
		Return(pgconn.NewCommandTag("UPDATE 0"), nil)
	// No Execute, no FinishTask expected.

	runtime := NewRuntime(store, fakeClock{now: fixedTime}, nil, nil, worker)
	require.NoError(t, runtime.runWorker(context.Background(), worker))
	assert.Zero(t, worker.executed, "Execute must not run when the task was not claimed")
}

func TestRuntime_RunWorker_SuccessFinishesTask(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	worker := newStubWorker()
	worker.outcome = WorkerOutcome{DeletedCount: 4}

	gomock.InOrder(
		store.EXPECT().
			ClaimTask(gomock.Any(), gomock.Any()).
			Return(pgconn.NewCommandTag("UPDATE 1"), nil),
		store.EXPECT().
			FinishTask(gomock.Any(), gomock.Any()).
			Return(nil),
	)

	runtime := NewRuntime(store, fakeClock{now: fixedTime}, nil, nil, worker)
	require.NoError(t, runtime.runWorker(context.Background(), worker))
	assert.Equal(t, 1, worker.executed)
}

func TestRuntime_RunWorker_FailureRecordsAndAnnotates(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	worker := newStubWorker()
	execErr := stderrors.New("boom")
	worker.execErr = execErr

	gomock.InOrder(
		store.EXPECT().
			ClaimTask(gomock.Any(), gomock.Any()).
			Return(pgconn.NewCommandTag("UPDATE 1"), nil),
		store.EXPECT().
			FinishTask(gomock.Any(), gomock.Any()).
			Return(nil),
	)

	runtime := NewRuntime(store, fakeClock{now: fixedTime}, nil, nil, worker)
	err := runtime.runWorker(context.Background(), worker)
	require.Error(t, err)
	require.ErrorIs(t, err, execErr, "the execution error must be preserved as the cause")

	meta := metaOf(t, err)
	assert.Equal(t, worker.name, meta["task"])
	assert.Equal(t, string(worker.stage), meta["stage"])
}

func TestRuntime_RunWorker_FinishSuccessErrorAnnotatesDeletedCount(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	worker := newStubWorker()
	worker.outcome = WorkerOutcome{DeletedCount: 9}

	gomock.InOrder(
		store.EXPECT().
			ClaimTask(gomock.Any(), gomock.Any()).
			Return(pgconn.NewCommandTag("UPDATE 1"), nil),
		store.EXPECT().
			FinishTask(gomock.Any(), gomock.Any()).
			Return(apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError))),
	)

	runtime := NewRuntime(store, fakeClock{now: fixedTime}, nil, nil, worker)
	err := runtime.runWorker(context.Background(), worker)
	require.Error(t, err)

	meta := metaOf(t, err)
	assert.Equal(t, worker.name, meta["task"])
	assert.Equal(t, string(apperrors.StageFinishSuccess), meta["stage"])
	assert.Equal(t, 9, meta["deleted_count"])
}

func TestRuntime_RunAllWorkers_ReportsErrorToChannel(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	store := NewMockStore(ctrl)
	worker := newStubWorker()

	store.EXPECT().
		ClaimTask(gomock.Any(), gomock.Any()).
		Return(pgconn.NewCommandTag("UPDATE 0"), apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError)))

	errCh := make(chan error, 1)
	runtime := NewRuntime(store, fakeClock{now: fixedTime}, nil, errCh, worker)
	runtime.runAllWorkers(context.Background())

	select {
	case err := <-errCh:
		require.Error(t, err)
		assert.Equal(t, worker.name, metaOf(t, err)["task"])
	default:
		t.Fatal("expected the claim error to be reported to the error channel")
	}
}
