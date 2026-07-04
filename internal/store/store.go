// Package store is the postgres-backed persistence implementation.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/audit"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/workers"
)

// Queries is the backend-neutral query contract implemented by the sqlc adapter.
type Queries interface {
	CountUsers(ctx context.Context) (int64, error)
	InsertUser(ctx context.Context, arg auth.InsertUserParams) error
	InsertAccount(ctx context.Context, arg auth.InsertAccountParams) error
	InsertOrganisation(ctx context.Context, arg auth.InsertOrganisationParams) error
	InsertOrganisationMember(ctx context.Context, arg auth.InsertOrganisationMemberParams) error
	FindUserByEmail(ctx context.Context, email string) (auth.UserRow, error)
	FindPasswordAccountByUserID(ctx context.Context, userID string) (auth.AccountRow, error)
	FindUserByProviderAccount(ctx context.Context, provider, providerAccountID string) (auth.UserRow, error)
	ListOrganisationsForUser(ctx context.Context, userID string) ([]auth.OrganisationRow, error)
	FindOrganisationBySlugForUser(ctx context.Context, userID, slug string) (auth.OrganisationRow, error)
	FindOrganisationByIDForUser(ctx context.Context, userID, organisationID string) (auth.OrganisationRow, error)
	UpdateUserPasswordHash(ctx context.Context, arg auth.UpdateUserPasswordHashParams) error
	InsertVerificationToken(ctx context.Context, arg auth.InsertVerificationTokenParams) error
	FindVerificationToken(ctx context.Context, tokenHash, purpose string) (auth.VerificationTokenRow, error)
	ConsumeVerificationToken(ctx context.Context, arg auth.ConsumeVerificationTokenParams) error
	SaveTwoFactor(ctx context.Context, arg auth.SaveTwoFactorParams) error
	FindTwoFactorByUserID(ctx context.Context, userID string) (auth.TwoFactor, error)
	ConfirmTwoFactor(ctx context.Context, arg auth.UseTwoFactorParams) error
	MarkTwoFactorUsed(ctx context.Context, arg auth.UseTwoFactorParams) error
	DisableTwoFactor(ctx context.Context, userID string) error
	ListSSOProviderSettings(ctx context.Context) ([]auth.SSOProviderSettingRow, error)
	UpsertSSOProviderSetting(ctx context.Context, arg auth.UpsertSSOProviderSettingParams) error
	InsertSession(ctx context.Context, arg auth.InsertSessionParams) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (auth.SessionRow, error)
	RevokeSessionByTokenHash(ctx context.Context, arg auth.RevokeSessionParams) error
	RevokeSessionsForUser(ctx context.Context, arg auth.RevokeSessionsForUserParams) error
	DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error)
	EnsureDefaultSettings(ctx context.Context, now time.Time) error
	GetInstanceName(ctx context.Context) (string, error)
	EnsureTask(ctx context.Context, arg workers.TaskParams) error
	ClaimTask(ctx context.Context, arg workers.ClaimTaskParams) (int64, error)
	FinishTask(ctx context.Context, arg workers.FinishTaskParams) error
	InsertAuditLog(ctx context.Context, arg audit.InsertAuditLogParams) error
	ListAuditLogs(ctx context.Context, arg audit.ListAuditLogsParams) ([]audit.AuditLogRow, error)
}

// DBTX is the common interface satisfied by both *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store owns a database connection or transaction and executes queries through the sqlc adapter.
type Store struct {
	db   *sql.DB
	tx   *sql.Tx
	q    Queries
	newQ func(DBTX) Queries
	name string
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Open creates a PostgreSQL store, runs migrations, and seeds default settings.
func Open(ctx context.Context, dsn string) (*Store, error) {
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, err
	}
	goose.SetBaseFS(migrationsFS)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, closeWithErr(db, err)
	}

	s := &Store{db: db, q: newQueries(db), newQ: newQueries, name: "postgres"}
	if err := s.EnsureDefaultSettings(ctx); err != nil {
		return nil, closeWithErr(db, err)
	}
	return s, nil
}

// New opens a root store backed by db.
func New(db *sql.DB, name string, newQ func(DBTX) Queries) *Store {
	return &Store{db: db, q: newQ(db), newQ: newQ, name: name}
}

func newTxStore(tx *sql.Tx, name string, newQ func(DBTX) Queries) *Store {
	return &Store{tx: tx, q: newQ(tx), newQ: newQ, name: name}
}

// Query returns the active backend query implementation.
func (s *Store) Query() Queries {
	return s.q
}

// DB exposes the underlying database handle for callers that need the raw sqlc queries.
func (s *Store) DB() *sql.DB {
	return s.db
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

func closeWithErr(c interface{ Close() error }, cause error) error {
	if closeErr := c.Close(); closeErr != nil {
		return errors.Join(cause, closeErr)
	}
	return cause
}
