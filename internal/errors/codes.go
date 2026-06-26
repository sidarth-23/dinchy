package errors

const (
	CodeAuthInvalidCredentials = "auth.invalid_credentials"
	CodeAuthSetupCompleted     = "auth.setup_completed"
	CodeAuthUnauthenticated    = "auth.unauthenticated"

	CodeSecurityHTTPSRequired = "security.https_required"
	CodeSecurityCSRFFailed    = "security.csrf_failed"

	CodeRequestValidationFailed = "request.validation_failed"

	CodeConfigLoadFailed       = "config.load_failed"
	CodeConfigValidationFailed = "config.validation_failed"

	CodeServerInternalError = "server.internal_error"
)
