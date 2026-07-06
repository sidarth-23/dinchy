package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

func invitationRow(rowID, organisationID, email, role, status, tokenHash, invitedByUserID string, expiresAt time.Time, acceptedAt sql.NullTime) sqlcgen.FindOrganisationInvitationByTokenRow {
	return sqlcgen.FindOrganisationInvitationByTokenRow{
		ID:              id.MustParse(rowID),
		OrganisationID:  id.MustParse(organisationID),
		Email:           email,
		Role:            role,
		Status:          status,
		TokenHash:       tokenHash,
		ExpiresAt:       expiresAt,
		InvitedByUserID: id.MustParse(invitedByUserID),
		AcceptedAt:      acceptedAt,
	}
}

func TestCreateInvitation_SendsEmailAndStoresToken(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{configured: true}
	svc, store := newServiceWithSender(t, sender)

	session := &SessionWithUser{
		UserID:           testUserID,
		Email:            "owner@example.com",
		OrganisationID:   testOrganisationID,
		OrganisationName: "Default",
		OrganisationSlug: "default",
		Role:             RoleOwner,
	}

	store.EXPECT().FindUserByEmail(gomock.Any(), "invitee@example.com").Return(sqlcgen.FindUserByEmailRow{}, sql.ErrNoRows)
	store.EXPECT().FindPendingOrganisationInvitationByEmail(gomock.Any(), gomock.Any()).Return(sqlcgen.FindPendingOrganisationInvitationByEmailRow{}, sql.ErrNoRows)
	store.EXPECT().InsertOrganisationInvitation(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in sqlcgen.InsertOrganisationInvitationParams) error {
		assert.Equal(t, "invitee@example.com", in.Email)
		assert.Equal(t, string(RoleMember), in.Role)
		assert.Equal(t, testUserID, in.InvitedByUserID.String())
		assert.True(t, in.ExpiresAt.After(fixedTime))
		assert.NotEmpty(t, in.TokenHash)
		return nil
	})

	invitation, err := svc.CreateInvitation(testCtx, session, "invitee@example.com", "member", "127.0.0.1", "ua")
	require.NoError(t, err)
	require.NotNil(t, invitation)
	assert.Equal(t, "invitee@example.com", invitation.Email)
	assert.Len(t, sender.sent, 1)
	assert.Equal(t, "invitee@example.com", sender.sent[0].To)
	assert.Contains(t, sender.sent[0].Text, "Accept the invitation")
}

func TestAcceptInvitation_CreatesUserAndSession(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	rawToken := "invite-token"
	tokenHash := security.HashToken(rawToken)
	invitationID := testVerificationTokenID

	store.EXPECT().
		FindOrganisationInvitationByToken(gomock.Any(), tokenHash).
		Return(invitationRow(invitationID, testOrganisationID, "invitee@example.com", string(RoleAdmin), string(InvitationStatusPending), tokenHash, testUserID, fixedTime.Add(time.Hour), sql.NullTime{}), nil)
	store.EXPECT().FindUserByEmail(gomock.Any(), "invitee@example.com").Return(sqlcgen.FindUserByEmailRow{}, sql.ErrNoRows)
	store.EXPECT().InsertUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in sqlcgen.InsertUserParams) error {
		assert.Equal(t, "invitee@example.com", in.Email)
		assert.True(t, in.EmailVerifiedAt.Valid)
		return nil
	})
	store.EXPECT().FindPasswordAccountByUserID(gomock.Any(), gomock.Any()).Return(sqlcgen.FindPasswordAccountByUserIDRow{}, sql.ErrNoRows)
	store.EXPECT().InsertAccount(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().FindOrganisationByIDForUser(gomock.Any(), gomock.Any()).Return(sqlcgen.FindOrganisationByIDForUserRow{}, sql.ErrNoRows)
	store.EXPECT().InsertOrganisationMember(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().ConsumeOrganisationInvitation(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)

	token, err := svc.AcceptInvitation(testCtx, rawToken, "Invitee", "password123", "127.0.0.1", "ua")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestAcceptInvitation_InvalidToken(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().FindOrganisationInvitationByToken(gomock.Any(), security.HashToken("bad-token")).Return(sqlcgen.FindOrganisationInvitationByTokenRow{}, sql.ErrNoRows)

	_, err := svc.AcceptInvitation(testCtx, "bad-token", "Invitee", "password123", "", "")
	require.ErrorIs(t, err, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvitationInvalid)))
}
