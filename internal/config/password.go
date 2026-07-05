package config

type PasswordHashVersion string

const PasswordHashVersionV1 PasswordHashVersion = "v1"

type PasswordHashAlgorithm string

const PasswordHashAlgorithmArgon2ID PasswordHashAlgorithm = "argon2id"

type PasswordHashParamKey string

const (
	PasswordHashParamMemory  PasswordHashParamKey = "m"
	PasswordHashParamTime    PasswordHashParamKey = "t"
	PasswordHashParamThreads PasswordHashParamKey = "p"
)

type PasswordHashParams struct {
	Memory  uint32
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen int
}

func DefaultPasswordHashParams() PasswordHashParams {
	return PasswordHashParams{
		Memory:  64 * 1024,
		Time:    2,
		Threads: 4,
		KeyLen:  32,
		SaltLen: 16,
	}
}
