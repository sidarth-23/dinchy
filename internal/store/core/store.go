package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

// Queries is the backend-neutral query contract implemented by each sqlc adapter.
type Queries interface {
	CountUsers(ctx context.Context) (int64, error)
	InsertUser(ctx context.Context, arg InsertUserParams) error
	InsertAccount(ctx context.Context, arg InsertAccountParams) error
	InsertOrganisation(ctx context.Context, arg InsertOrganisationParams) error
	InsertOrganisationMember(ctx context.Context, arg InsertOrganisationMemberParams) error
	FindUserByEmail(ctx context.Context, email string) (UserRow, error)
	FindPasswordAccountByUserID(ctx context.Context, userID string) (AccountRow, error)
	FindUserByProviderAccount(ctx context.Context, provider, providerAccountID string) (UserRow, error)
	ListOrganisationsForUser(ctx context.Context, userID string) ([]OrganisationRow, error)
	FindOrganisationBySlugForUser(ctx context.Context, userID, slug string) (OrganisationRow, error)
	FindOrganisationByIDForUser(ctx context.Context, userID, organisationID string) (OrganisationRow, error)
	UpdateUserPasswordHash(ctx context.Context, arg UpdateUserPasswordHashParams) error
	InsertVerificationToken(ctx context.Context, arg InsertVerificationTokenParams) error
	FindVerificationToken(ctx context.Context, tokenHash, purpose string) (VerificationTokenRow, error)
	ConsumeVerificationToken(ctx context.Context, arg ConsumeVerificationTokenParams) error
	SaveTwoFactor(ctx context.Context, arg SaveTwoFactorParams) error
	FindTwoFactorByUserID(ctx context.Context, userID string) (TwoFactorRow, error)
	ConfirmTwoFactor(ctx context.Context, arg UseTwoFactorParams) error
	MarkTwoFactorUsed(ctx context.Context, arg UseTwoFactorParams) error
	DisableTwoFactor(ctx context.Context, userID string) error
	ListSSOProviderSettings(ctx context.Context) ([]SSOProviderSettingRow, error)
	UpsertSSOProviderSetting(ctx context.Context, arg UpsertSSOProviderSettingParams) error
	InsertSession(ctx context.Context, arg InsertSessionParams) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (SessionRow, error)
	RevokeSessionByTokenHash(ctx context.Context, arg RevokeSessionParams) error
	RevokeSessionsForUser(ctx context.Context, arg RevokeSessionsForUserParams) error
	DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error)
	EnsureDefaultSettings(ctx context.Context, now time.Time) error
	GetInstanceName(ctx context.Context) (string, error)
	EnsureTask(ctx context.Context, arg TaskParams) error
	ClaimTask(ctx context.Context, arg ClaimTaskParams) (int64, error)
	FinishTask(ctx context.Context, arg FinishTaskParams) error
	InsertAuditLog(ctx context.Context, arg InsertAuditLogParams) error
	ListAuditLogs(ctx context.Context, arg ListAuditLogsParams) ([]AuditLogRow, error)
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
