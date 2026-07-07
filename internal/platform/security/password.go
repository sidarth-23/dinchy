package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/sidarth-23/dinchy/internal/config"
)

type parsedPasswordHash struct {
	params config.PasswordHashParams
	salt   []byte
	hash   []byte
}

func HashPassword(password string) (string, error) {
	params := config.DefaultPasswordHashParams()
	salt := make([]byte, params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, params.Time, params.Memory, params.Threads, params.KeyLen)
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
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

func VerifyPassword(password, encoded string) bool {
	spec, ok := parsePasswordHash(encoded)
	if !ok {
		return false
	}
	sum := argon2.IDKey([]byte(password), spec.salt, spec.params.Time, spec.params.Memory, spec.params.Threads, spec.params.KeyLen)
	return subtle.ConstantTimeCompare(sum, spec.hash) == 1
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
	if err != nil || len(salt) != params.SaltLen {
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

	params := config.PasswordHashParams{KeyLen: config.DefaultPasswordHashParams().KeyLen, SaltLen: config.DefaultPasswordHashParams().SaltLen}
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
