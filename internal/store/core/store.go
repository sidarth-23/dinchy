package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/bootstrap"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// Queries is the backend-neutral query contract implemented by each sqlc adapter.
type Queries interface {
	CountUsers(ctx context.Context) (int64, error)
	InsertUser(ctx context.Context, arg InsertUserParams) error
	FindUserByEmail(ctx context.Context, email string) (UserRow, error)
	InsertSession(ctx context.Context, arg InsertSessionParams) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (SessionRow, error)
	RevokeSessionByTokenHash(ctx context.Context, arg RevokeSessionParams) error
	DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error)
	EnsureDefaultSettings(ctx context.Context, now time.Time) error
	GetInstanceName(ctx context.Context) (string, error)
	EnsureTask(ctx context.Context, arg TaskParams) error
	ClaimTask(ctx context.Context, arg ClaimTaskParams) (int64, error)
	FinishTask(ctx context.Context, arg FinishTaskParams) error
}

// Store owns a database connection or transaction and executes queries through a backend-neutral adapter.
type Store struct {
	db   *sql.DB
	tx   *sql.Tx
	q    Queries
	newQ func(DBTX) Queries
	name string
}

// New opens a root store backed by db.
func New(db *sql.DB, name string, newQ func(DBTX) Queries) *Store {
	return &Store{db: db, q: newQ(db), newQ: newQ, name: name}
}

func newTxStore(tx *sql.Tx, name string, newQ func(DBTX) Queries) *Store {
	return &Store{tx: tx, q: newQ(tx), newQ: newQ, name: name}
}

// PingContext verifies the database connection is alive.
func (s *Store) PingContext(ctx context.Context) error {
	if s.db == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("%s cannot ping a transaction-scoped store", s.name)), apperrors.WithOperation(apperrors.OperationPingContext))
	}
	return s.db.PingContext(ctx)
}

// Close shuts down the database connection.
func (s *Store) Close() error {
	if s.db == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("%s cannot close a transaction-scoped store", s.name)), apperrors.WithOperation(apperrors.OperationClose))
	}
	return s.db.Close()
}

// WithTx executes fn in a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	if s.tx != nil {
		if err := fn(s); err != nil {
			return apperrors.Annotate(err,
				apperrors.WithOperation(apperrors.OperationWithTx),
				apperrors.WithStage(apperrors.StageTxPassthrough),
			)
		}
		return nil
	}

	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationBeginTx))
	}

	txStore := newTxStore(sqlTx, s.name, s.newQ)
	if err := fn(txStore); err != nil {
		if rbErr := sqlTx.Rollback(); rbErr != nil {
			return errors.Join(
				apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationWithTx), apperrors.WithStage(apperrors.StageBody)),
				apperrors.Annotate(rbErr, apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationWithTx), apperrors.WithStage(apperrors.StageBody))
	}

	if err := sqlTx.Commit(); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationCommit))
	}
	return nil
}

// EnsureDefaultSettings seeds singleton settings if they are missing.
func (s *Store) EnsureDefaultSettings(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.q.EnsureDefaultSettings(ctx, now); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationEnsureDefaultSettings))
	}
	return nil
}

// Bootstrap returns whether setup is required and the configured instance name.
func (s *Store) Bootstrap(ctx context.Context) (bootstrap.BootstrapState, error) {
	count, err := s.q.CountUsers(ctx)
	if err != nil {
		return bootstrap.BootstrapState{}, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationCountUsers))
	}
	name, err := s.q.GetInstanceName(ctx)
	if err != nil {
		return bootstrap.BootstrapState{}, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationGetInstanceName))
	}
	return bootstrap.BootstrapState{SetupRequired: count == 0, InstanceName: name}, nil
}

// CreateFirstUser inserts the initial admin user inside a transaction.
func (s *Store) CreateFirstUser(ctx context.Context, in auth.CreateUserInput) (auth.User, error) {
	var u auth.User
	err := s.WithTx(ctx, func(tx *Store) error {
		count, err := tx.q.CountUsers(ctx)
		if err != nil {
			return apperrors.Annotate(err,
				apperrors.WithOperation(apperrors.OperationCountUsers),
				apperrors.WithStage(apperrors.StageSetupFirstUser),
			)
		}
		if count > 0 {
			return apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", int(count))))
		}
		if err := tx.q.InsertUser(ctx, InsertUserParams{
			ID:           in.ID,
			Email:        in.Email,
			PasswordHash: in.PasswordHash,
			DisplayName:  in.DisplayName,
			Role:         string(auth.RoleAdmin),
			CreatedAt:    in.Now.UTC(),
			UpdatedAt:    in.Now.UTC(),
		}); err != nil {
			return apperrors.Annotate(err,
				apperrors.WithOperation(apperrors.OperationInsertUser),
				apperrors.WithStage(apperrors.StageSetupFirstUser),
			)
		}
		u = auth.User{
			ID:           in.ID,
			Email:        in.Email,
			DisplayName:  in.DisplayName,
			Role:         auth.RoleAdmin,
			PasswordHash: in.PasswordHash,
		}
		return nil
	})
	return u, err
}

// FindUserByEmail looks up an active user by email.
func (s *Store) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	row, err := s.q.FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindUserByEmail))
	}
	return &auth.User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		DisplayName:  row.DisplayName,
		Role:         auth.Role(row.Role),
	}, nil
}

// CreateSession inserts a new session record and returns its ID.
func (s *Store) CreateSession(ctx context.Context, in session.CreateSessionInput) (session.Session, error) {
	if err := s.q.InsertSession(ctx, InsertSessionParams{
		ID:            in.ID,
		UserID:        in.UserID,
		TokenHash:     in.TokenHash,
		IpAddress:     in.IP,
		UserAgent:     in.UserAgent,
		LastSeenAt:    in.Now.UTC(),
		IdleExpiresAt: in.IdleExpiresAt.UTC(),
		ExpiresAt:     in.ExpiresAt.UTC(),
		CreatedAt:     in.Now.UTC(),
		UpdatedAt:     in.Now.UTC(),
	}); err != nil {
		return session.Session{}, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationCreateSession))
	}
	return session.Session{ID: in.ID}, nil
}

// GetSessionByTokenHash retrieves an active session with its owner's user info.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*session.SessionWithUser, error) {
	row, err := s.q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationGetSessionByTokenHash))
	}
	return &session.SessionWithUser{
		SessionID:     row.ID,
		UserID:        row.UserID,
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		Role:          session.Role(row.Role),
		IdleExpiresAt: row.IdleExpiresAt.UTC(),
		ExpiresAt:     row.ExpiresAt.UTC(),
		RevokedAt:     row.RevokedAt,
	}, nil
}

// RevokeSessionByTokenHash marks the matching session as revoked.
func (s *Store) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	now := time.Now().UTC()
	if err := s.q.RevokeSessionByTokenHash(ctx, RevokeSessionParams{
		RevokedAt: now,
		UpdatedAt: now,
		TokenHash: tokenHash,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationRevokeSessionByTokenHash), apperrors.WithTokenHash(apperrors.TokenHash(tokenHash)))
	}
	return nil
}

// DeleteEndedSessionsOlderThan deletes ended sessions that were updated before the cutoff.
func (s *Store) DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	n, err := s.q.DeleteEndedSessionsOlderThan(ctx, olderThan.UTC())
	if err != nil {
		return 0, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationDeleteEndedSessionsOlderThan))
	}
	return n, nil
}

// EnsureTask registers a task by name if it does not already exist.
func (s *Store) EnsureTask(ctx context.Context, name string, intervalSeconds int64, now time.Time) error {
	taskID, err := uuid.NewV7()
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(err), apperrors.WithOperation(apperrors.OperationEnsureTask), apperrors.WithTask(apperrors.Task(name)))
	}
	if err := s.q.EnsureTask(ctx, TaskParams{
		ID:                      taskID.String(),
		TaskName:                name,
		ScheduleIntervalSeconds: intervalSeconds,
		NextRunAt:               now.UTC(),
		UpdatedAt:               now.UTC(),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationEnsureTask), apperrors.WithTask(apperrors.Task(name)))
	}
	return nil
}

// ClaimTask atomically acquires the lease on a task that is due to run.
func (s *Store) ClaimTask(ctx context.Context, taskName, owner string, leaseUntil, now time.Time) (bool, error) {
	n, err := s.q.ClaimTask(ctx, ClaimTaskParams{
		LeaseOwner:     owner,
		LeaseExpiresAt: leaseUntil.UTC(),
		LastRunAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
		TaskName:       taskName,
		NextRunAt:      now.UTC(),
	})
	if err != nil {
		return false, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationClaimTask), apperrors.WithTask(apperrors.Task(taskName)))
	}
	return n > 0, nil
}

// FinishTask records the outcome of a completed task run.
func (s *Store) FinishTask(ctx context.Context, taskName string, now time.Time, ok bool, errCode, errMsg string, nextRun time.Time) error {
	status := "failed"
	if ok {
		status = "ok"
	}
	if err := s.q.FinishTask(ctx, FinishTaskParams{
		LastFinishedAt:   now.UTC(),
		NextRunAt:        nextRun.UTC(),
		LastStatus:       status,
		LastErrorCode:    errCode,
		LastErrorMessage: errMsg,
		UpdatedAt:        now.UTC(),
		TaskName:         taskName,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFinishTask), apperrors.WithTask(apperrors.Task(taskName)))
	}
	return nil
}
