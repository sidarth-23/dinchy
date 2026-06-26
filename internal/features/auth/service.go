// Package auth handles password hashing, session issuance, and session validation.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
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

const (
	passwordHashVersion   = "v1"
	passwordHashAlgorithm = "argon2id"
	passwordHashSaltLen   = 16
	passwordHashMemory    = 64 * 1024
	passwordHashTime      = 2
	passwordHashThreads   = 4
	passwordHashKeyLen    = 32

	legacyPasswordHashSalt = "dinchy-static-salt-phase1"
	legacyPasswordHashTime = 1
	legacyPasswordHashMem  = 64 * 1024
	legacyPasswordHashThrd = 4
	legacyPasswordHashLen  = 32
)

type passwordHashParams struct {
	memory  uint32
	time    uint32
	threads uint8
	keyLen  uint32
}

var currentPasswordHashParams = passwordHashParams{
	memory:  passwordHashMemory,
	time:    passwordHashTime,
	threads: passwordHashThreads,
	keyLen:  passwordHashKeyLen,
}

type parsedPasswordHash struct {
	params passwordHashParams
	salt   []byte
	hash   []byte
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
func (s *Service) Session(ctx context.Context, rawToken string) (*session.SessionWithUser, error) {
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

func (s *Service) newSession(ctx context.Context, userID, ip, ua string) (string, error) {
	token, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowNewSession),
			apperrors.WithStage(apperrors.StageGenerateToken),
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
			apperrors.WithFlow(apperrors.FlowNewSession),
			apperrors.WithStage(apperrors.StageCreateSession),
		)
	}
	return token, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, passwordHashSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, currentPasswordHashParams.time, currentPasswordHashParams.memory, currentPasswordHashParams.threads, currentPasswordHashParams.keyLen)
	return formatPasswordHash(salt, sum, currentPasswordHashParams), nil
}

func verifyPassword(password, encoded string) bool {
	if strings.HasPrefix(encoded, passwordHashVersion+"$") {
		spec, ok := parsePasswordHash(encoded)
		if !ok {
			return false
		}
		sum := argon2.IDKey([]byte(password), spec.salt, spec.params.time, spec.params.memory, spec.params.threads, spec.params.keyLen)
		return subtle.ConstantTimeCompare(sum, spec.hash) == 1
	}

	legacySalt := sha256.Sum256([]byte(legacyPasswordHashSalt))
	sum := argon2.IDKey([]byte(password), legacySalt[:], legacyPasswordHashTime, legacyPasswordHashMem, legacyPasswordHashThrd, legacyPasswordHashLen)
	decoded, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(sum, decoded) == 1
}

func needsPasswordHashUpgrade(encoded string) bool {
	return !strings.HasPrefix(encoded, passwordHashVersion+"$")
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

func formatPasswordHash(salt, hash []byte, params passwordHashParams) string {
	return fmt.Sprintf(
		"%s$%s$m=%d,t=%d,p=%d$%s$%s",
		passwordHashVersion,
		passwordHashAlgorithm,
		params.memory,
		params.time,
		params.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func parsePasswordHash(encoded string) (parsedPasswordHash, bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 {
		return parsedPasswordHash{}, false
	}
	if parts[0] != passwordHashVersion || parts[1] != passwordHashAlgorithm {
		return parsedPasswordHash{}, false
	}
	params, ok := parsePasswordHashParams(parts[2])
	if !ok {
		return parsedPasswordHash{}, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != passwordHashSaltLen {
		return parsedPasswordHash{}, false
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(hash) != int(params.keyLen) {
		return parsedPasswordHash{}, false
	}
	return parsedPasswordHash{
		params: params,
		salt:   salt,
		hash:   hash,
	}, true
}

func parsePasswordHashParams(raw string) (passwordHashParams, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return passwordHashParams{}, false
	}

	var params passwordHashParams
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return passwordHashParams{}, false
		}
		v, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return passwordHashParams{}, false
		}
		switch kv[0] {
		case "m":
			params.memory = uint32(v)
		case "t":
			params.time = uint32(v)
		case "p":
			params.threads = uint8(v)
		default:
			return passwordHashParams{}, false
		}
	}
	if params.memory == 0 || params.time == 0 || params.threads == 0 {
		return passwordHashParams{}, false
	}
	params.keyLen = passwordHashKeyLen
	return params, true
}
