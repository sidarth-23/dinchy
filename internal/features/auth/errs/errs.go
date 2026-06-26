package errs

import "errors"

// Sentinels emitted by the auth feature and mapped to public API errors at the transport boundary.
var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrSetupCompleted     = errors.New("setup already completed")
)
