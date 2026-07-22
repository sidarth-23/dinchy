// Package jobs provides a durable, Postgres-backed background job queue (River)
// for on-demand work that must survive restarts and is retried until it succeeds.
//
// It complements internal/workers, which runs ephemeral in-memory recurring
// schedules (gocron): use workers for periodic polling that can be missed and
// jobs for one-off work that must not be lost.
package jobs

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
)

// New builds the River client that runs durable background jobs. Workers must be
// registered on the provided registry before the client is created; the caller
// owns starting and stopping the returned client.
func New(pool *pgxpool.Pool, logger *slog.Logger, cfg config.JobsConfig, workers *river.Workers) (*river.Client[pgx.Tx], error) {
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:  logger,
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.MaxWorkers}},
		Workers: workers,
	})
	if err != nil {
		return nil, apperrors.Annotate(err)
	}
	return client, nil
}

// Enqueuer inserts durable jobs. EnqueueTx enrolls the insert in an existing
// transaction so the job is enqueued if and only if that transaction commits.
type Enqueuer interface {
	Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error
	EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error
}

// NewEnqueuer returns an Enqueuer backed by the River client.
func NewEnqueuer(client *river.Client[pgx.Tx]) Enqueuer {
	return &clientEnqueuer{client: client}
}

type clientEnqueuer struct {
	client *river.Client[pgx.Tx]
}

func (e *clientEnqueuer) Enqueue(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) error {
	if _, err := e.client.Insert(ctx, args, opts); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}

func (e *clientEnqueuer) EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error {
	if _, err := e.client.InsertTx(ctx, tx, args, opts); err != nil {
		return apperrors.Annotate(err)
	}
	return nil
}
