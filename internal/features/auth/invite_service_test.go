package auth

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/access/session"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

func invitationRow(rowID, organizationID, emailAddress, role, status, tokenHash, invitedByUserID string, expiresAt time.Time, acceptedAt pgtype.Timestamptz) sqlcgen.FindOrganizationInvitationByTokenRow {
	return sqlcgen.FindOrganizationInvitationByTokenRow{
		ID:              id.MustParse(rowID),
		OrganizationID:  id.MustParse(organizationID),
		Email:           emailAddress,
		Role:            role,
		Status:          status,
		TokenHash:       tokenHash,
		ExpiresAt:       sqltype.Timestamptz(expiresAt),
		InvitedByUserID: id.MustParse(invitedByUserID),
		AcceptedAt:      acceptedAt,
	}
}

func TestCreateInvitation_SendsEmailAndStoresToken(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{configured: true}
	svc, store := newServiceWithSender(t, sender)

	principal := &session.Principal{
		UserID:           testUserID,
		Email:            "owner@example.com",
		OrganizationID:   testOrganizationID,
		OrganizationName: "Default",
		OrganizationSlug: "default",
		Role:             permission.RoleAdmin,
		Permissions:      []permission.Permission{permission.AuthInvitationsCreate},
	}

	store.EXPECT().FindUserByEmail(gomock.Any(), "invitee@example.com").Return(sqlcgen.FindUserByEmailRow{}, pgx.ErrNoRows)
	store.EXPECT().FindPendingOrganizationInvitationByEmail(gomock.Any(), gomock.Any()).Return(sqlcgen.FindPendingOrganizationInvitationByEmailRow{}, pgx.ErrNoRows)
	store.EXPECT().InsertOrganizationInvitation(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in sqlcgen.InsertOrganizationInvitationParams) error {
		assert.Equal(t, "invitee@example.com", in.Email)
		assert.Equal(t, string(permission.RoleMember), in.Role)
		assert.Equal(t, testUserID, in.InvitedByUserID.String())
		assert.True(t, sqltype.TimeValue(in.ExpiresAt).After(fixedTime))
		assert.NotEmpty(t, in.TokenHash)
		return nil
	})

	invitation, err := svc.CreateInvitation(testCtx, principal, "invitee@example.com", permission.RoleMember, "127.0.0.1", "ua")
	require.NoError(t, err)
	require.NotNil(t, invitation)
	assert.Equal(t, "invitee@example.com", invitation.Email)
	enqueuer := svc.Jobs.(*fakeEnqueuer)
	require.Len(t, enqueuer.enqueued, 1)
	args, ok := enqueuer.enqueued[0].(email.SendEmailArgs)
	require.True(t, ok)
	assert.Equal(t, "invitee@example.com", args.To)
	assert.Contains(t, args.Text, "Default")
	assert.Contains(t, args.Text, "https://app.test/accept-invitation?token=")
	assert.NotEmpty(t, args.HTML)
}

func TestAcceptInvitation_CreatesUserAndSession(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	rawToken := "invite-token"
	tokenHash := security.HashToken(rawToken)
	invitationID := testVerificationTokenID

	store.EXPECT().
		FindOrganizationInvitationByToken(gomock.Any(), tokenHash).
		Return(invitationRow(invitationID, testOrganizationID, "invitee@example.com", string(permission.RoleAdmin), string(InvitationStatusPending), tokenHash, testUserID, fixedTime.Add(time.Hour), pgtype.Timestamptz{}), nil)
	store.EXPECT().FindUserByEmail(gomock.Any(), "invitee@example.com").Return(sqlcgen.FindUserByEmailRow{}, pgx.ErrNoRows)
	store.EXPECT().InsertUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, in sqlcgen.InsertUserParams) error {
		assert.Equal(t, "invitee@example.com", in.Email)
		assert.True(t, in.EmailVerifiedAt.Valid)
		return nil
	})
	store.EXPECT().FindPasswordAccountByUserID(gomock.Any(), gomock.Any()).Return(sqlcgen.FindPasswordAccountByUserIDRow{}, pgx.ErrNoRows)
	store.EXPECT().InsertAccount(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().FindOrganizationByIDForUser(gomock.Any(), gomock.Any()).Return(sqlcgen.FindOrganizationByIDForUserRow{}, pgx.ErrNoRows)
	store.EXPECT().InsertOrganizationMember(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().ConsumeOrganizationInvitation(gomock.Any(), gomock.Any()).Return(nil)
	store.EXPECT().InsertSession(gomock.Any(), gomock.Any()).Return(nil)

	token, err := svc.AcceptInvitation(testCtx, rawToken, "Invitee", "password123", "127.0.0.1", "ua")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestAcceptInvitation_InvalidToken(t *testing.T) {
	t.Parallel()
	svc, store := newTestService(t)

	store.EXPECT().FindOrganizationInvitationByToken(gomock.Any(), security.HashToken("bad-token")).Return(sqlcgen.FindOrganizationInvitationByTokenRow{}, pgx.ErrNoRows)

	_, err := svc.AcceptInvitation(testCtx, "bad-token", "Invitee", "password123", "", "")
	require.ErrorIs(t, err, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthInvitationInvalid)))
}
