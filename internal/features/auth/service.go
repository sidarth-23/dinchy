// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/session"
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
	hash := hashPassword(password)
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
			apperrors.WithMeta("flow", "setup_first_user"),
			apperrors.WithMeta("stage", "create_first_user"),
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
			apperrors.WithMeta("flow", "login"),
			apperrors.WithMeta("stage", "find_user"),
		)
	}
	if u == nil || !verifyPassword(password, u.PasswordHash) {
		return "", apperrors.New(http.StatusUnauthorized, i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	return s.newSession(ctx, u.ID, ip, ua)
}

// Session validates a raw token and returns the associated session and user if valid.
// Returns nil without error for expired, revoked, or missing tokens.
func (s *Service) Session(ctx context.Context, rawToken string) (*session.SessionWithUser, error) {
	if rawToken == "" {
		return nil, nil
	}
	sess, err := s.store.GetSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil || sess == nil {
		return nil, apperrors.Annotate(err,
			apperrors.WithMeta("flow", "session"),
			apperrors.WithMeta("stage", "get_session"),
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
			apperrors.WithMeta("flow", "logout"),
			apperrors.WithMeta("stage", "revoke_session"),
		)
	}
	return nil
}

func (s *Service) newSession(ctx context.Context, userID, ip, ua string) (string, error) {
	token, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithMeta("flow", "new_session"),
			apperrors.WithMeta("stage", "generate_token"),
		)
	}
	now := s.clock.Now()
	_, err = s.store.CreateSession(ctx, session.CreateSessionInput{
		ID:            s.idg.New(),
		UserID:        userID,
		TokenHash:     tokenHash,
		IP:            ip,
		UserAgent:     ua,
		Now:           now,
		IdleExpiresAt: now.Add(30 * time.Minute),
		ExpiresAt:     now.Add(7 * 24 * time.Hour),
	})
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithMeta("flow", "new_session"),
			apperrors.WithMeta("stage", "create_session"),
		)
	}
	return token, nil
}

func hashPassword(password string) string {
	salt := sha256.Sum256([]byte("dinchy-static-salt-phase1"))
	sum := argon2.IDKey([]byte(password), salt[:], 1, 64*1024, 4, 32)
	return base64.RawStdEncoding.EncodeToString(sum)
}

func verifyPassword(password, encoded string) bool {
	return hashPassword(password) == encoded
}

func generateSessionToken() (raw, tokenHash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashToken(raw), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
