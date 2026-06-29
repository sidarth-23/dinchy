// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
	"strings"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
)

// Service provides authentication operations backed by a persistent store.
type Service struct {
	store Store
	idg   *id.Generator
	clock clock.Clock
}

// NewService creates an auth service with the given store, ID generator, and clock.
func NewService(s Store, idg *id.Generator, clk clock.Clock) *Service {
	return &Service{store: s, idg: idg, clock: clk}
}

// SetupFirstUser creates the initial admin account and issues a session token.
// Returns the structured setup-completed error if any user already exists.
func (s *Service) SetupFirstUser(ctx context.Context, email, displayName, password, ip, ua string) (string, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowSetupFirstUser),
			apperrors.WithStage(apperrors.StageSetupFirstUser),
		)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	now := s.clock.Now()
	u, err := s.store.CreateFirstUser(ctx, CreateUserInput{
		ID:           s.idg.New(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		Now:          now,
	})
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowSetupFirstUser),
			apperrors.WithStage(apperrors.StageCreateFirstUser),
		)
	}
	return s.newSession(ctx, u.ID, ip, ua)
}

// Login validates credentials and issues a session token.
// Returns the structured invalid-credentials error if the email is not found or the password is wrong.
func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := s.store.FindUserByEmail(ctx, email)
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowLogin),
			apperrors.WithStage(apperrors.StageFindUser),
		)
	}
	if u == nil || !verifyPassword(password, u.PasswordHash) {
		return "", apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	if needsPasswordHashUpgrade(u.PasswordHash) {
		newHash, err := hashPassword(password)
		if err != nil {
			return "", apperrors.Annotate(err,
				apperrors.WithFlow(apperrors.FlowLogin),
				apperrors.WithStage(apperrors.StageLogin),
			)
		}
		if err := s.store.UpdateUserPasswordHash(ctx, UpdateUserPasswordHashInput{
			UserID:       u.ID,
			PasswordHash: newHash,
			Now:          s.clock.Now(),
		}); err != nil {
			return "", apperrors.Annotate(err,
				apperrors.WithFlow(apperrors.FlowLogin),
				apperrors.WithStage(apperrors.StageLogin),
			)
		}
	}
	return s.newSession(ctx, u.ID, ip, ua)
}

// Session validates a raw token and returns the associated session and user if valid.
// Returns nil without error for expired, revoked, or missing tokens.
func (s *Service) Session(ctx context.Context, rawToken string) (*SessionWithUser, error) {
	if rawToken == "" {
		return nil, nil
	}
	sess, err := s.store.GetSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowSession),
			apperrors.WithStage(apperrors.StageGetSession),
		)
	}
	now := s.clock.Now()
	if sess.RevokedAt.Valid || now.After(sess.IdleExpiresAt) || now.After(sess.ExpiresAt) {
		return nil, nil
	}
	return sess, nil
}

// Logout revokes the session associated with rawToken.
func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	err := s.store.RevokeSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowLogout),
			apperrors.WithStage(apperrors.StageRevokeSession),
		)
	}
	return nil
}
