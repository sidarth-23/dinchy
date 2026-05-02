package i18n

import "reflect"

// Messages defines every translatable message in the application.
// Each field's msg tag carries the wire-format key used in API responses.
type Messages struct {
	AuthInvalidCredentials string `msg:"auth.invalid_credentials"`
	AuthSetupCompleted     string `msg:"auth.setup_completed"`
	AuthUnauthenticated    string `msg:"auth.unauthenticated"`
	SecurityHTTPSRequired  string `msg:"security.https_required"`
	SecurityCSRFFailed     string `msg:"security.csrf_failed"`
	ServerInternalError    string `msg:"server.internal_error"`
}

// MsgFunc selects a single message from a Messages struct.
type MsgFunc func(Messages) string

// msgTagIndex is a sentinel Messages value where each field holds its own msg tag.
// Built once at init time via reflect — zero cost on the hot path.
var msgTagIndex = func() Messages {
	var m Messages
	v := reflect.ValueOf(&m).Elem()
	t := v.Type()
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("msg")
		v.Field(i).SetString(tag)
	}
	return m
}()

// MsgCode returns the wire-format msg tag for the field selected by fn.
// Example: MsgCode(func(m Messages) string { return m.AuthInvalidCredentials })
// returns "auth.invalid_credentials".
func MsgCode(fn MsgFunc) string {
	return fn(msgTagIndex)
}
