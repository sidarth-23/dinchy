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

	"golang.org/x/crypto/argon2"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
)

const (
	passwordHashVersion   = "v1"
	passwordHashAlgorithm = "argon2id"
	passwordHashSaltLen   = 16
	passwordHashMemory    = 64 * 1024
	passwordHashTime      = 2
	passwordHashThreads   = 4
	passwordHashKeyLen    = 32
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

func (s *Service) newSession(ctx context.Context, userID, organisationID, ip, ua string) (string, error) {
	token, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowNewSession),
			apperrors.WithStage(apperrors.StageGenerateToken),
		)
	}
	now := s.clock.Now()
	_, err = s.store.CreateSession(ctx, CreateSessionInput{
		ID:             s.idg.New(),
		UserID:         userID,
		OrganisationID: organisationID,
		TokenHash:      tokenHash,
		IP:             ip,
		UserAgent:      ua,
		Now:            now,
		IdleExpiresAt:  now.Add(s.authConfig.SessionIdleTimeout),
		ExpiresAt:      now.Add(s.authConfig.SessionMaxLifetime),
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
	spec, ok := parsePasswordHash(encoded)
	if !ok {
		return false
	}
	sum := argon2.IDKey([]byte(password), spec.salt, spec.params.time, spec.params.memory, spec.params.threads, spec.params.keyLen)
	return subtle.ConstantTimeCompare(sum, spec.hash) == 1
}

func generateSessionToken() (raw, tokenHash string, err error) {
	raw, err = newRandomToken(32)
	if err != nil {
		return "", "", err
	}
	return raw, hashToken(raw), nil
}

func newRandomToken(size int) (string, error) {
	buf := make([]byte, 32)
	if size > 0 {
		buf = make([]byte, size)
	}
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
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
