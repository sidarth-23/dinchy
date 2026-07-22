package config

// Frontend route paths used to build outbound email call-to-action links.
const (
	// AcceptInvitationPath is the frontend route that accepts an organization invitation.
	AcceptInvitationPath = "/accept-invitation"
	// ResetPasswordPath is the frontend route that completes a password reset.
	ResetPasswordPath = "/reset-password"
)

// Links carries the public base URL and the frontend route paths used to build
// outbound email call-to-action links. It is the single source features read to
// assemble those links.
type Links struct {
	BaseURL              string
	AcceptInvitationPath string
	ResetPasswordPath    string
}

// NewLinks builds Links from the public base URL and the well-known frontend paths.
func NewLinks(baseURL string) Links {
	return Links{
		BaseURL:              baseURL,
		AcceptInvitationPath: AcceptInvitationPath,
		ResetPasswordPath:    ResetPasswordPath,
	}
}
