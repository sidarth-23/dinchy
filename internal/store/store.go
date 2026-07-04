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
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/store/types"
)

// Queries is the backend-neutral query contract implemented by the sqlc adapter.
type Queries interface {
	CountUsers(ctx context.Context) (int64, error)
	InsertUser(ctx context.Context, arg types.InsertUserParams) error
	InsertAccount(ctx context.Context, arg types.InsertAccountParams) error
	InsertOrganisation(ctx context.Context, arg types.InsertOrganisationParams) error
	InsertOrganisationMember(ctx context.Context, arg types.InsertOrganisationMemberParams) error
	FindUserByEmail(ctx context.Context, email string) (types.UserRow, error)
	FindPasswordAccountByUserID(ctx context.Context, userID string) (types.AccountRow, error)
	FindUserByProviderAccount(ctx context.Context, provider, providerAccountID string) (types.UserRow, error)
	ListOrganisationsForUser(ctx context.Context, userID string) ([]types.OrganisationRow, error)
	FindOrganisationBySlugForUser(ctx context.Context, userID, slug string) (types.OrganisationRow, error)
	FindOrganisationByIDForUser(ctx context.Context, userID, organisationID string) (types.OrganisationRow, error)
	UpdateUserPasswordHash(ctx context.Context, arg types.UpdateUserPasswordHashParams) error
	InsertVerificationToken(ctx context.Context, arg types.InsertVerificationTokenParams) error
	FindVerificationToken(ctx context.Context, tokenHash, purpose string) (types.VerificationTokenRow, error)
	ConsumeVerificationToken(ctx context.Context, arg types.ConsumeVerificationTokenParams) error
	SaveTwoFactor(ctx context.Context, arg types.SaveTwoFactorParams) error
	FindTwoFactorByUserID(ctx context.Context, userID string) (types.TwoFactorRow, error)
	ConfirmTwoFactor(ctx context.Context, arg types.UseTwoFactorParams) error
	MarkTwoFactorUsed(ctx context.Context, arg types.UseTwoFactorParams) error
	DisableTwoFactor(ctx context.Context, userID string) error
	ListSSOProviderSettings(ctx context.Context) ([]types.SSOProviderSettingRow, error)
	UpsertSSOProviderSetting(ctx context.Context, arg types.UpsertSSOProviderSettingParams) error
	InsertSession(ctx context.Context, arg types.InsertSessionParams) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (types.SessionRow, error)
	RevokeSessionByTokenHash(ctx context.Context, arg types.RevokeSessionParams) error
	RevokeSessionsForUser(ctx context.Context, arg types.RevokeSessionsForUserParams) error
	DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error)
	EnsureDefaultSettings(ctx context.Context, now time.Time) error
	GetInstanceName(ctx context.Context) (string, error)
	EnsureTask(ctx context.Context, arg types.TaskParams) error
	ClaimTask(ctx context.Context, arg types.ClaimTaskParams) (int64, error)
	FinishTask(ctx context.Context, arg types.FinishTaskParams) error
	InsertAuditLog(ctx context.Context, arg types.InsertAuditLogParams) error
	ListAuditLogs(ctx context.Context, arg types.ListAuditLogsParams) ([]types.AuditLogRow, error)
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
