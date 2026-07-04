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

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
)

var currentPasswordHashParams = config.DefaultPasswordHashParams()

func (s *Service) newSession(ctx context.Context, userID, organisationID, ip, ua string) (string, error) {
	token, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowNewSession),
			apperrors.WithStage(apperrors.StageGenerateToken),
		)
	}
	now := s.clock.Now()
	err = s.store.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   mustParseUUID(s.idg.New()),
		UserID:               mustParseUUID(userID),
		ActiveOrganisationID: mustParseUUID(organisationID),
		TokenHash:            tokenHash,
		IpAddress:            ip,
		UserAgent:            ua,
		LastSeenAt:           now.UTC(),
		IdleExpiresAt:        now.Add(s.authConfig.SessionIdleTimeout).UTC(),
		ExpiresAt:            now.Add(s.authConfig.SessionMaxLifetime).UTC(),
		CreatedAt:            now.UTC(),
		UpdatedAt:            now.UTC(),
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
	salt := make([]byte, currentPasswordHashParams.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, currentPasswordHashParams.Time, currentPasswordHashParams.Memory, currentPasswordHashParams.Threads, currentPasswordHashParams.KeyLen)
	return formatPasswordHash(salt, sum, currentPasswordHashParams), nil
}

func verifyPassword(password, encoded string) bool {
	spec, ok := parsePasswordHash(encoded)
	if !ok {
		return false
	}
	sum := argon2.IDKey([]byte(password), spec.salt, spec.params.Time, spec.params.Memory, spec.params.Threads, spec.params.KeyLen)
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

func formatPasswordHash(salt, hash []byte, params config.PasswordHashParams) string {
	return fmt.Sprintf(
		"%s$%s$%s=%d,%s=%d,%s=%d$%s$%s",
		config.PasswordHashVersionV1,
		config.PasswordHashAlgorithmArgon2ID,
		config.PasswordHashParamMemory,
		params.Memory,
		config.PasswordHashParamTime,
		params.Time,
		config.PasswordHashParamThreads,
		params.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func parsePasswordHash(encoded string) (parsedPasswordHash, bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 {
		return parsedPasswordHash{}, false
	}
	if config.PasswordHashVersion(parts[0]) != config.PasswordHashVersionV1 || config.PasswordHashAlgorithm(parts[1]) != config.PasswordHashAlgorithmArgon2ID {
		return parsedPasswordHash{}, false
	}
	params, ok := parsePasswordHashParams(parts[2])
	if !ok {
		return parsedPasswordHash{}, false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != currentPasswordHashParams.SaltLen {
		return parsedPasswordHash{}, false
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(hash) != int(params.KeyLen) {
		return parsedPasswordHash{}, false
	}
	return parsedPasswordHash{
		params: params,
		salt:   salt,
		hash:   hash,
	}, true
}

func parsePasswordHashParams(raw string) (config.PasswordHashParams, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return config.PasswordHashParams{}, false
	}

	params := config.PasswordHashParams{SaltLen: currentPasswordHashParams.SaltLen, KeyLen: currentPasswordHashParams.KeyLen}
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return config.PasswordHashParams{}, false
		}
		v, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return config.PasswordHashParams{}, false
		}
		switch config.PasswordHashParamKey(kv[0]) {
		case config.PasswordHashParamMemory:
			params.Memory = uint32(v)
		case config.PasswordHashParamTime:
			params.Time = uint32(v)
		case config.PasswordHashParamThreads:
			params.Threads = uint8(v)
		default:
			return config.PasswordHashParams{}, false
		}
	}
	if params.Memory == 0 || params.Time == 0 || params.Threads == 0 {
		return config.PasswordHashParams{}, false
	}
	return params, true
}
