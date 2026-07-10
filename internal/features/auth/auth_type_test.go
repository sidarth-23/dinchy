package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sidarth-23/dinchy/internal/access/permission"
)

// Request resolvers normalize input at the transport boundary before validation
// errors are returned, so downstream handlers and services receive canonical
// values. huma.Context is unused by these resolvers, so nil is passed.

func TestLoginBodyResolve_NormalizesFields(t *testing.T) {
	t.Parallel()
	body := LoginBody{Email: "  USER@EXAMPLE.COM  ", OrganisationSlug: "  acme  ", TOTPCode: "  123456  "}
	require.Nil(t, body.Resolve(nil))
	assert.Equal(t, "user@example.com", body.Email)
	assert.Equal(t, "acme", body.OrganisationSlug)
	assert.Equal(t, "123456", body.TOTPCode)
}

func TestSetupBodyResolve_NormalizesEmailAndDisplayName(t *testing.T) {
	t.Parallel()
	body := SetupBody{Email: "  ADMIN@EXAMPLE.COM  ", DisplayName: "  Ada Lovelace  "}
	require.Nil(t, body.Resolve(nil))
	assert.Equal(t, "admin@example.com", body.Email)
	assert.Equal(t, "Ada Lovelace", body.DisplayName)
}

func TestForgotPasswordBodyResolve_NormalizesEmail(t *testing.T) {
	t.Parallel()
	body := ForgotPasswordBody{Email: "  USER@EXAMPLE.COM  "}
	require.Nil(t, body.Resolve(nil))
	assert.Equal(t, "user@example.com", body.Email)
}

func TestCreateInvitationBodyResolve_NormalizesEmail(t *testing.T) {
	t.Parallel()
	body := CreateInvitationBody{Email: "  INVITEE@EXAMPLE.COM  ", Role: permission.RoleMember}
	require.Nil(t, body.Resolve(nil))
	assert.Equal(t, "invitee@example.com", body.Email)
	assert.Equal(t, permission.RoleMember, body.Role)
}

func TestAcceptInvitationBodyResolve_TrimsDisplayName(t *testing.T) {
	t.Parallel()
	body := AcceptInvitationBody{DisplayName: "  Ada  "}
	require.Nil(t, body.Resolve(nil))
	assert.Equal(t, "Ada", body.DisplayName)
}

func TestSelectOrganisationBodyResolve_TrimsSlug(t *testing.T) {
	t.Parallel()
	body := SelectOrganisationBody{OrganisationSlug: "  acme  "}
	require.Nil(t, body.Resolve(nil))
	assert.Equal(t, "acme", body.OrganisationSlug)
}

func TestTOTPConfirmBodyResolve_TrimsCode(t *testing.T) {
	t.Parallel()
	body := TOTPConfirmBody{Code: "  123456  "}
	require.Nil(t, body.Resolve(nil))
	assert.Equal(t, "123456", body.Code)
}

func TestSSOStartInResolve_TrimsSlug(t *testing.T) {
	t.Parallel()
	in := SSOStartIn{OrganisationSlug: "  acme  "}
	require.Nil(t, in.Resolve(nil))
	assert.Equal(t, "acme", in.OrganisationSlug)
}
