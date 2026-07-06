package workers

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

//go:generate mockgen -self_package=github.com/sidarth-23/dinchy/internal/workers -destination=store_mockdata_test.go -package=workers . Store

// Store is the data access contract required by the worker runtime.
type Store interface {
	EnsureTask(ctx context.Context, arg sqlcgen.EnsureTaskParams) error
	ClaimTask(ctx context.Context, arg sqlcgen.ClaimTaskParams) (pgconn.CommandTag, error)
	FinishTask(ctx context.Context, arg sqlcgen.FinishTaskParams) error
	DeleteEndedSessionsOlderThan(ctx context.Context, arg sqlcgen.DeleteEndedSessionsOlderThanParams) (pgconn.CommandTag, error)
}

func taskIDForName(taskName string) uuid.UUID {
	return uuid.NewSHA1(uuid.Nil, []byte(taskName))
}
