package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	"github.com/sidarth-23/dinchy/internal/access/session"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

const invitationStatusPending = "pending"

func invitationFromFindRow(row sqlcgen.FindOrganisationInvitationByTokenRow) *Invitation {
	if row.ID == uuid.Nil {
		return nil
	}
	invitation := &Invitation{
		ID:              row.ID.String(),
		OrganisationID:  row.OrganisationID.String(),
		Email:           row.Email,
		Role:            permission.Role(row.Role),
		Status:          InvitationStatus(row.Status),
		ExpiresAt:       sqltype.TimeValue(row.ExpiresAt),
		InvitedByUserID: row.InvitedByUserID.String(),
	}
	if row.AcceptedAt.Valid {
		invitation.AcceptedAt = sqltype.TimeValue(row.AcceptedAt)
		invitation.AcceptedAtValid = true
	}
	return invitation
}

// CreateInvitation issues and emails an organization invitation, requiring invitation permission and rejecting duplicates.
func (s *Service) CreateInvitation(ctx context.Context, inviter *session.Principal, emailAddress string, invitationRole permission.Role, ip, userAgent string) (*Invitation, error) {
	if !s.Mailer.Configured() {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeNotificationEmailNotConfigured), apperrors.WithCause(email.ErrNotConfigured))
	}
	if inviter == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthUnauthenticated))
	}
	if !inviter.HasPermission(permission.AuthInvitationsCreate) {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAccountAuthForbidden))
	}
	now := s.Clock.Now().UTC()
	organisationID := id.MustParse(inviter.OrganisationID)

	if userRow, err := s.store.FindUserByEmail(ctx, emailAddress); err == nil {
		if user := userFromFindUserRow(userRow); user != nil {
			if _, membershipErr := s.store.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
				UserID: id.MustParse(user.ID),
				ID:     organisationID,
			}); membershipErr == nil {
				return nil, apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthInvitationExists))
			} else if !errors.Is(membershipErr, pgx.ErrNoRows) {
				return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindOrganisation), apperrors.WithCause(membershipErr))
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindUser), apperrors.WithCause(err))
	}

	if pendingInvitation, err := s.store.FindPendingOrganisationInvitationByEmail(ctx, sqlcgen.FindPendingOrganisationInvitationByEmailParams{
		OrganisationID: organisationID,
		Email:          emailAddress,
	}); err == nil {
		if invitationFromFindRow(sqlcgen.FindOrganisationInvitationByTokenRow(pendingInvitation)) != nil {
			return nil, apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthInvitationExists))
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindInvitation), apperrors.WithCause(err))
	}

	rawToken, err := security.RandomToken(32)
	if err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationGenerateToken), apperrors.WithCause(err))
	}
	invitationID := s.IDGenerator.New()
	expiresAt := now.Add(s.authConfig.InviteLifetime)
	if err := s.store.InsertOrganisationInvitation(ctx, sqlcgen.InsertOrganisationInvitationParams{
		ID:              id.MustParse(invitationID),
		OrganisationID:  organisationID,
		Email:           emailAddress,
		Role:            string(invitationRole),
		Status:          invitationStatusPending,
		TokenHash:       security.HashToken(rawToken),
		ExpiresAt:       sqltype.Timestamptz(expiresAt),
		InvitedByUserID: id.MustParse(inviter.UserID),
		CreatedAt:       sqltype.Timestamptz(now),
		UpdatedAt:       sqltype.Timestamptz(now),
	}); err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationCreateInvitation), apperrors.WithCause(err))
	}
	if err := s.Mailer.Send(ctx, emailAddress, s.invitationContent(inviter.OrganisationName, string(invitationRole), rawToken)); err != nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationSendEmail), apperrors.WithCause(err))
	}
	return &Invitation{
		ID:              invitationID,
		OrganisationID:  organisationID.String(),
		Email:           emailAddress,
		Role:            invitationRole,
		Status:          InvitationStatusPending,
		ExpiresAt:       expiresAt,
		InvitedByUserID: inviter.UserID,
	}, nil
}

// AcceptInvitation consumes a valid invitation token, creating or updating the user and organization membership in one transaction, and returns a session token.
func (s *Service) AcceptInvitation(ctx context.Context, token, displayName, password, ip, userAgent string) (string, error) {
	if s.beginTx == nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(errors.New("transaction opener is required for invitation acceptance")))
	}
	tx, err := s.beginTx(ctx)
	if err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationBeginTx), apperrors.WithCause(err))
	}
	now := s.Clock.Now().UTC()
	invitationRow, err := tx.queries.FindOrganisationInvitationByToken(ctx, security.HashToken(token))
	if err != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindInvitation), apperrors.WithCause(err)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
			)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthInvitationInvalid))
		}
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindInvitation), apperrors.WithCause(err))
	}
	invitation := invitationFromFindRow(invitationRow)
	if invitation == nil || invitation.Status != InvitationStatusPending || invitation.AcceptedAtValid || now.After(invitation.ExpiresAt) {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthInvitationInvalid)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
			)
		}
		return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthInvitationInvalid))
	}

	emailAddress := invitation.Email
	userDisplayName := displayName
	if userDisplayName == "" {
		userDisplayName = emailAddress
	}
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationPasswordHash), apperrors.WithCause(err)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
			)
		}
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationPasswordHash), apperrors.WithCause(err))
	}

	userRow, err := tx.queries.FindUserByEmail(ctx, emailAddress)
	var user *User
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindUser), apperrors.WithCause(err)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
			)
		}
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindUser), apperrors.WithCause(err))
	}
	if err == nil {
		user = userFromFindUserRow(userRow)
		if user == nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthInvitationInvalid)),
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
				)
			}
			return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthInvitationInvalid))
		}
		if !user.EmailVerified {
			if err := tx.queries.UpdateUserEmailVerifiedAt(ctx, sqlcgen.UpdateUserEmailVerifiedAtParams{
				ID:              id.MustParse(user.ID),
				EmailVerifiedAt: sqltype.Timestamptz(now),
				UpdatedAt:       sqltype.Timestamptz(now),
			}); err != nil {
				if rbErr := tx.rollback(); rbErr != nil {
					return "", errors.Join(
						apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationUpdateEmailVerified), apperrors.WithCause(err)),
						apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
					)
				}
				return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationUpdateEmailVerified), apperrors.WithCause(err))
			}
		}
	} else {
		userID := s.IDGenerator.New()
		if err := tx.queries.InsertUser(ctx, sqlcgen.InsertUserParams{
			ID:              id.MustParse(userID),
			Email:           emailAddress,
			DisplayName:     userDisplayName,
			EmailVerifiedAt: sqltype.Timestamptz(now),
			CreatedAt:       sqltype.Timestamptz(now),
			UpdatedAt:       sqltype.Timestamptz(now),
		}); err != nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationAccept), apperrors.WithCause(err)),
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
				)
			}
			return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationAccept), apperrors.WithCause(err))
		}
		user = &User{ID: userID, Email: emailAddress, DisplayName: userDisplayName, EmailVerified: true}
	}

	if _, err := tx.queries.FindPasswordAccountByUserID(ctx, id.MustParse(user.ID)); err == nil {
		if err := tx.queries.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
			UserID:       id.MustParse(user.ID),
			PasswordHash: sqltype.Text(passwordHash),
			UpdatedAt:    sqltype.Timestamptz(now),
		}); err != nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationPasswordHash), apperrors.WithCause(err)),
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
				)
			}
			return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationPasswordHash), apperrors.WithCause(err))
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.queries.InsertAccount(ctx, sqlcgen.InsertAccountParams{
			ID:                id.MustParse(s.IDGenerator.New()),
			UserID:            id.MustParse(user.ID),
			Provider:          string(AccountProviderPassword),
			ProviderAccountID: emailAddress,
			PasswordHash:      sqltype.Text(passwordHash),
			CreatedAt:         sqltype.Timestamptz(now),
			UpdatedAt:         sqltype.Timestamptz(now),
		}); err != nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationAccept), apperrors.WithCause(err)),
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
				)
			}
			return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationAccept), apperrors.WithCause(err))
		}
	} else {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindAccount), apperrors.WithCause(err)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
			)
		}
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindAccount), apperrors.WithCause(err))
	}

	if memberRow, err := tx.queries.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
		UserID: id.MustParse(user.ID),
		ID:     id.MustParse(invitation.OrganisationID),
	}); err == nil {
		_ = memberRow
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.queries.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{
			ID:             id.MustParse(s.IDGenerator.New()),
			OrganisationID: id.MustParse(invitation.OrganisationID),
			UserID:         id.MustParse(user.ID),
			Role:           string(invitation.Role),
			CreatedAt:      sqltype.Timestamptz(now),
			UpdatedAt:      sqltype.Timestamptz(now),
		}); err != nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationCreateInvitation), apperrors.WithCause(err)),
					apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
				)
			}
			return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationCreateInvitation), apperrors.WithCause(err))
		}
	} else {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindOrganisation), apperrors.WithCause(err)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
			)
		}
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationFindOrganisation), apperrors.WithCause(err))
	}

	if err := tx.queries.ConsumeOrganisationInvitation(ctx, sqlcgen.ConsumeOrganisationInvitationParams{
		ID:         id.MustParse(invitation.ID),
		AcceptedAt: sqltype.Timestamptz(now),
		UpdatedAt:  sqltype.Timestamptz(now),
	}); err != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationConsumeInvitation), apperrors.WithCause(err)),
				apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationRollback), apperrors.WithCause(rbErr)),
			)
		}
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationConsumeInvitation), apperrors.WithCause(err))
	}
	if err := tx.commit(); err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthInvitationCommit), apperrors.WithCause(err))
	}

	token, sessionErr := s.sessions.Create(ctx, user.ID, invitation.OrganisationID, ip, userAgent)
	if sessionErr != nil {
		return "", sessionErr
	}
	return token, nil
}
