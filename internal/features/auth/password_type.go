package auth

import "github.com/sidarth-23/dinchy/internal/config"

type parsedPasswordHash struct {
	params config.PasswordHashParams
	salt   []byte
	hash   []byte
}
