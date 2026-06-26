package i18n

// Messages defines every translatable message in the application.
// Each field name maps to a stable error code via the `msg` tag.
type Messages struct {
	AuthInvalidCredentials  string `msg:"auth.invalid_credentials"`
	AuthSetupCompleted      string `msg:"auth.setup_completed"`
	AuthUnauthenticated     string `msg:"auth.unauthenticated"`
	SecurityHTTPSRequired   string `msg:"security.https_required"`
	SecurityCSRFFailed      string `msg:"security.csrf_failed"`
	RequestValidationFailed string `msg:"request.validation_failed"`
	ConfigLoadFailed        string `msg:"config.load_failed"`
	ConfigValidationFailed  string `msg:"config.validation_failed"`
	ServerInternalError     string `msg:"server.internal_error"`
}
