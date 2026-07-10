package session

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// Store is the persistence surface the session service depends on.
type Store interface {
	InsertSession(ctx context.Context, arg sqlcgen.InsertSessionParams) error
	GetSessionByTokenHash(ctx context.Context, tokenHash string) (sqlcgen.GetSessionByTokenHashRow, error)
	RevokeSessionByTokenHash(ctx context.Context, arg sqlcgen.RevokeSessionByTokenHashParams) error
	RevokeSessionsForUser(ctx context.Context, arg sqlcgen.RevokeSessionsForUserParams) error
}

// Service owns session token creation, resolution, revocation, and cookie naming.
type Service struct {
	store  Store
	idg    *id.Generator
	clock  clock.Clock
	config config.SessionConfig
}

// NewService builds a session Service.
func NewService(store Store, idg *id.Generator, clk clock.Clock, sessionConfig config.SessionConfig) *Service {
	return &Service{store: store, idg: idg, clock: clk, config: sessionConfig}
}

// SessionCookieName returns the configured name of the session cookie.
func (s *Service) SessionCookieName() string {
	return s.config.SessionCookieName
}

// Session resolves a raw token to its active session, returning nil when the token is empty, unknown, revoked, or expired.
func (s *Service) Session(ctx context.Context, rawToken string) (*Principal, error) {
	if rawToken == "" {
		return nil, nil
	}
	row, err := s.store.GetSessionByTokenHash(ctx, security.HashToken(rawToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSession), apperrors.WithStage(apperrors.StageGetSession))
	}
	principal := FromGetSessionRow(row)
	now := s.clock.Now()
	if principal.RevokedAt.Valid || now.After(principal.IdleExpiresAt) || now.After(principal.ExpiresAt) {
		return nil, nil
	}
	return principal, nil
}

// Create issues a new session token for the given user and organization.
func (s *Service) Create(ctx context.Context, userID, organisationID, ip, userAgent string) (string, error) {
	token := s.idg.New()
	now := s.clock.Now().UTC()
	if err := s.store.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   id.MustParse(token),
		UserID:               id.MustParse(userID),
		ActiveOrganisationID: id.MustParse(organisationID),
		TokenHash:            security.HashToken(token),
		IpAddress:            ip,
		UserAgent:            userAgent,
		IdleExpiresAt:        sqltype.Timestamptz(now.Add(s.config.SessionIdleTimeout)),
		ExpiresAt:            sqltype.Timestamptz(now.Add(s.config.SessionMaxLifetime)),
		CreatedAt:            sqltype.Timestamptz(now),
		UpdatedAt:            sqltype.Timestamptz(now),
	}); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowNewSession), apperrors.WithStage(apperrors.StageCreateSession))
	}
	return token, nil
}

// Logout revokes the session identified by the raw token and returns the resolved principal.
func (s *Service) Logout(ctx context.Context, rawToken string) (*Principal, error) {
	if rawToken == "" {
		return nil, nil
	}
	principal, err := s.Session(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if err := s.store.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{RevokedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now), TokenHash: security.HashToken(rawToken)}); err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageRevokeSession))
	}
	return principal, nil
}

// RevokeForUser revokes every active session for the given user.
func (s *Service) RevokeForUser(ctx context.Context, userID string) error {
	now := s.clock.Now()
	if err := s.store.RevokeSessionsForUser(ctx, sqlcgen.RevokeSessionsForUserParams{UserID: id.MustParse(userID), RevokedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithOperation(apperrors.OperationRevokeSessionsForUser), apperrors.WithStage(apperrors.StageRevokeSession))
	}
	return nil
}
