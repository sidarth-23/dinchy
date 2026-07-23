package errors_test

import (
	stdErrors "errors"
	"fmt"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
)

// ExampleBadRequest builds a 400 AppError from a stable i18n code and inspects
// its deterministic HTTP status and machine-readable code.
func ExampleBadRequest() {
	err := apperrors.BadRequest(i18n.Msg(i18n.CodeTransportSecurityCSRFFailed))

	fmt.Println(err.Status())
	fmt.Println(err.Code())
	// Output:
	// 400
	// transport.security.csrf_failed
}

// ExampleNew builds an AppError through the generic constructor for an explicit
// status and inspects its status and code.
func ExampleNew() {
	err := apperrors.New(401, i18n.Msg(i18n.CodeAccountAuthInvalidCredentials))

	fmt.Println(err.Status())
	fmt.Println(err.Code())
	// Output:
	// 401
	// account.auth.invalid_credentials
}

// ExampleInternal attaches an underlying cause to a 500 AppError and shows that
// the status is preserved while the cause remains reachable via errors.Unwrap.
func ExampleInternal() {
	cause := stdErrors.New("disk write failed")
	err := apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(cause))

	fmt.Println(err.Status())
	fmt.Println(stdErrors.Unwrap(err))
	// Output:
	// 500
	// disk write failed
}
