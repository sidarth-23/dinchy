package i18n

// En contains English translations for all Dinchy error codes.
var En = map[string]string{
	"auth.invalid_credentials": "Invalid email or password.",
	"auth.setup_completed":     "Setup has already been completed.",
	"auth.unauthenticated":     "Authentication required.",
	"security.https_required":  "This endpoint requires a secure (HTTPS) connection.",
	"security.csrf_failed":     "Missing or invalid CSRF token.",
	"server.internal_error":    "An unexpected error occurred.",
}
