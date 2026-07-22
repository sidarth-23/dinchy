// Package security hashes passwords and generates tokens for authentication.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

type parsedPasswordHash struct {
	params PasswordHashParams
	salt   []byte
	hash   []byte
}

// HashPassword returns an Argon2id hash of password encoded with its parameters and salt.
func HashPassword(password string) (string, error) {
	params := DefaultPasswordHashParams()
	salt := make([]byte, params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, params.Time, params.Memory, params.Threads, params.KeyLen)
	return fmt.Sprintf(
		"%s$%s$%s=%d,%s=%d,%s=%d$%s$%s",
		PasswordHashVersionV1,
		PasswordHashAlgorithmArgon2ID,
		PasswordHashParamMemory,
		params.Memory,
		PasswordHashParamTime,
		params.Time,
		PasswordHashParamThreads,
		params.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	), nil
}

// VerifyPassword reports whether password matches the Argon2id hash in encoded.
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
	if PasswordHashVersion(parts[0]) != PasswordHashVersionV1 || PasswordHashAlgorithm(parts[1]) != PasswordHashAlgorithmArgon2ID {
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

func parsePasswordHashParams(raw string) (PasswordHashParams, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return PasswordHashParams{}, false
	}

	params := PasswordHashParams{KeyLen: DefaultPasswordHashParams().KeyLen, SaltLen: DefaultPasswordHashParams().SaltLen}
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return PasswordHashParams{}, false
		}
		v, err := strconv.ParseUint(kv[1], 10, 32)
		if err != nil {
			return PasswordHashParams{}, false
		}
		switch PasswordHashParamKey(kv[0]) {
		case PasswordHashParamMemory:
			params.Memory = uint32(v)
		case PasswordHashParamTime:
			params.Time = uint32(v)
		case PasswordHashParamThreads:
			params.Threads = uint8(v)
		default:
			return PasswordHashParams{}, false
		}
	}
	if params.Memory == 0 || params.Time == 0 || params.Threads == 0 {
		return PasswordHashParams{}, false
	}
	return params, true
}
