package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sidarth-23/dinchy/internal/domain"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// CreateSession inserts a new session record and returns its ID.
func (s *Store) CreateSession(ctx context.Context, in domain.CreateSessionInput) (domain.Session, error) {
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
	return domain.Session{ID: in.ID}, err
}

// GetSessionByTokenHash retrieves an active session with its owner's user info.
func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*domain.SessionWithUser, error) {
	row, err := s.q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	idle, err := time.Parse(time.RFC3339Nano, row.IdleExpiresAt)
	if err != nil {
		return nil, err
	}
	exp, err := time.Parse(time.RFC3339Nano, row.ExpiresAt)
	if err != nil {
		return nil, err
	}
	var revokedAt sql.NullTime
	if row.RevokedAt.Valid {
		t, err := time.Parse(time.RFC3339Nano, row.RevokedAt.String)
		if err != nil {
			return nil, err
		}
		revokedAt = sql.NullTime{Time: t, Valid: true}
	}
	return &domain.SessionWithUser{
		SessionID:     row.ID,
		UserID:        row.UserID,
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		Role:          domain.Role(row.Role),
		IdleExpiresAt: idle,
		ExpiresAt:     exp,
		RevokedAt:     revokedAt,
	}, nil
}

// RevokeSessionByTokenHash marks the matching session as revoked.
func (s *Store) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	now := tsFormat(time.Now().UTC())
	return s.q.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{
		RevokedAt: sql.NullString{String: now, Valid: true},
		UpdatedAt: now,
		TokenHash: tokenHash,
	})
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
		return 0, err
	}
	return res.RowsAffected()
}
