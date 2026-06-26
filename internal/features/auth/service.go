// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/sidarth-23/dinchy/internal/domain"
	"github.com/sidarth-23/dinchy/internal/features/auth/errs"
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
// Returns ErrSetupCompleted if any user already exists.
func (s *Service) SetupFirstUser(ctx context.Context, email, displayName, password, ip, ua string) (string, error) {
	hash := hashPassword(password)
	email = strings.ToLower(strings.TrimSpace(email))
	now := s.clock.Now()
	u, err := s.store.CreateFirstUser(ctx, domain.CreateUserInput{
		ID:           s.idg.New(),
		Email:        email,
		PasswordHash: hash,
		DisplayName:  displayName,
		Now:          now,
	})
	if err != nil {
		if err.Error() == errs.ErrSetupCompleted.Error() {
			return "", errs.ErrSetupCompleted
		}
		return "", err
	}
	return s.newSession(ctx, u.ID, ip, ua)
}

// Login validates credentials and issues a session token.
// Returns ErrInvalidCredentials if the email is not found or the password is wrong.
func (s *Service) Login(ctx context.Context, email, password, ip, ua string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := s.store.FindUserByEmail(ctx, email)
	if err != nil {
		return "", err
	}
	if u == nil || !verifyPassword(password, u.PasswordHash) {
		return "", errs.ErrInvalidCredentials
	}
	return s.newSession(ctx, u.ID, ip, ua)
}

// Session validates a raw token and returns the associated session and user if valid.
// Returns nil without error for expired, revoked, or missing tokens.
func (s *Service) Session(ctx context.Context, rawToken string) (*domain.SessionWithUser, error) {
	if rawToken == "" {
		return nil, nil
	}
	sess, err := s.store.GetSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil || sess == nil {
		return nil, err
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
	return s.store.RevokeSessionByTokenHash(ctx, hashToken(rawToken))
}

func (s *Service) newSession(ctx context.Context, userID, ip, ua string) (string, error) {
	token, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", err
	}
	now := s.clock.Now()
	_, err = s.store.CreateSession(ctx, domain.CreateSessionInput{
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
		return "", err
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
