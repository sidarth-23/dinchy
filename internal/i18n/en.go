package i18n

// En contains English translations for all Dinchy messages.
var En = Messages{
	AuthInvalidCredentials:  "Invalid email or password.",
	AuthSetupCompleted:      "Setup has already been completed.",
	AuthUnauthenticated:     "Authentication required.",
	SecurityHTTPSRequired:   "This endpoint requires a secure (HTTPS) connection.",
	SecurityCSRFFailed:      "Missing or invalid CSRF token.",
	RequestValidationFailed: "Some fields need attention.",
	ConfigLoadFailed:        "Failed to load configuration.",
	ConfigValidationFailed:  "Configuration is invalid.",
	ServerInternalError:     "An unexpected error occurred.",
}
