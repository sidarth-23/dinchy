package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/features/session"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// CreateSession inserts a new session record and returns its ID.
func (s *Store) CreateSession(ctx context.Context, in session.CreateSessionInput) (session.Session, error) {
	now := tsFormat(in.Now)
	err := s.q.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:            in.ID,
		UserID:        in.UserID,
		TokenHash:     in.TokenHash,
		IpAddress:     in.IP,
		UserAgent:     in.UserAgent,
		LastSeenAt:    now,
		IdleExpiresAt: tsFormat(in.IdleExpiresAt),
		ExpiresAt:     tsFormat(in.ExpiresAt),
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		return session.Session{}, apperrors.Annotate(err,
			apperrors.WithOperation(apperrors.OperationCreateSession),
		)
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
		return nil, apperrors.Annotate(err,
			apperrors.WithOperation(apperrors.OperationGetSessionByTokenHash),
		)
	}
	idle, err := time.Parse(time.RFC3339Nano, row.IdleExpiresAt)
	if err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(err), apperrors.WithOperation(apperrors.OperationGetSessionByTokenHash), apperrors.WithField(apperrors.FieldName("idle_expires_at")))
	}
	exp, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(err), apperrors.WithOperation(apperrors.OperationGetSessionByTokenHash), apperrors.WithField(apperrors.FieldName("expires_at")))
	}
	var revokedAt sql.NullTime
	if row.RevokedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, row.RevokedAt.String)
		if err != nil {
			return nil, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(err), apperrors.WithOperation(apperrors.OperationGetSessionByTokenHash), apperrors.WithField(apperrors.FieldName("revoked_at")))
		}
		revokedAt = sql.NullTime{Time: t, Valid: true}
	}
	return &session.SessionWithUser{
		SessionID:     row.ID,
		UserID:        row.UserID,
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		Role:          auth.Role(row.Role),
		IdleExpiresAt: idle,
		ExpiresAt:     exp,
		RevokedAt:     revokedAt,
	}, nil
}

// RevokeSessionByTokenHash marks the matching session as revoked.
func (s *Store) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	now := tsFormat(time.Now().UTC())
	if err := s.q.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{
		RevokedAt: sql.NullString{String: now, Valid: true},
		UpdatedAt: now,
		TokenHash: tokenHash,
	}); err != nil {
		return apperrors.Annotate(err,
			apperrors.WithOperation(apperrors.OperationRevokeSessionByTokenHash),
			apperrors.WithTokenHash(apperrors.TokenHash(tokenHash)),
		)
	}
	return nil
}

// DeleteEndedSessionsOlderThan deletes sessions that have ended (revoked or expired)
// and were last updated before the given cutoff. Returns the number of rows deleted.
func (s *Store) DeleteEndedSessionsOlderThan(ctx context.Context, olderThan time.Time) (int64, error) {
	cutoff := tsFormat(olderThan)
	res, err := s.q.DeleteEndedSessionsOlderThan(ctx, sqlcgen.DeleteEndedSessionsOlderThanParams{
		ExpiresAt: cutoff,
		UpdatedAt: cutoff,
	})
	if err != nil {
		return 0, apperrors.Annotate(err,
			apperrors.WithOperation(apperrors.OperationDeleteEndedSessionsOlderThan),
		)
	}
	return res.RowsAffected()
}
