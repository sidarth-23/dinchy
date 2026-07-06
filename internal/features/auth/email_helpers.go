package auth

import "fmt"

func passwordResetEmailText(token string) string {
	return fmt.Sprintf("Use this password reset token before it expires:\n\n%s\n", token)
}

func invitationEmailText(organisationName, role, token string) string {
	return fmt.Sprintf(
		"You have been invited to join %s as %s.\n\nInvitation token:\n%s\n\nAccept the invitation with:\nPOST /auth/invitations/%s/accept\n\n",
		organisationName,
		role,
		token,
		token,
	)
}
