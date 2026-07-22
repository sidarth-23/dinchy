package security

// PasswordHashVersion identifies the encoded password hash format version.
type PasswordHashVersion string

// PasswordHashVersionV1 is the initial password hash encoding version.
const PasswordHashVersionV1 PasswordHashVersion = "v1"

// PasswordHashAlgorithm names the algorithm used to hash passwords.
type PasswordHashAlgorithm string

// PasswordHashAlgorithmArgon2ID selects the Argon2id password hashing algorithm.
const PasswordHashAlgorithmArgon2ID PasswordHashAlgorithm = "argon2id"

// PasswordHashParamKey is a short key used to encode a hash parameter in the hash string.
type PasswordHashParamKey string

// Encoded parameter keys for memory, time, and thread cost.
const (
	PasswordHashParamMemory  PasswordHashParamKey = "m"
	PasswordHashParamTime    PasswordHashParamKey = "t"
	PasswordHashParamThreads PasswordHashParamKey = "p"
)

// PasswordHashParams holds the Argon2id cost and length parameters.
type PasswordHashParams struct {
	Memory  uint32
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen int
}

// DefaultPasswordHashParams returns the default Argon2id hashing parameters.
func DefaultPasswordHashParams() PasswordHashParams {
	return PasswordHashParams{
		Memory:  64 * 1024,
		Time:    2,
		Threads: 4,
		KeyLen:  32,
		SaltLen: 16,
	}
}
