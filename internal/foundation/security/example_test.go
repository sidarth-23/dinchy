package security_test

import (
	"fmt"
	"log"

	"github.com/sidarth-23/dinchy/internal/foundation/security"
)

// ExampleHashToken shows that HashToken is deterministic: the same input always
// produces the same base64url-encoded SHA-256 digest, suitable for storage and lookup.
func ExampleHashToken() {
	fmt.Println(security.HashToken("example-token"))
	// Output: TRVmodffQqhRdFbWDqBu0oTlNc_kyVaqbuFy2835Rfc
}

// ExampleRandomToken shows generating a cryptographically random token. Its value
// changes on every call, so the result is not output-checked here.
func ExampleRandomToken() {
	token, err := security.RandomToken(32)
	if err != nil {
		log.Fatalf("generate random token: %v", err)
	}
	fmt.Println(token != "")
}
