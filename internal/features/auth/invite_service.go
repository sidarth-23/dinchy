package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
		Role:            Role(row.Role),
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

func (s *Service) CreateInvitation(ctx context.Context, inviter *SessionWithUser, emailAddress string, invitationRole Role, ip, userAgent string) (*Invitation, error) {
	if !s.mailer.Configured() {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeEmailNotConfigured), apperrors.WithCause(email.ErrNotConfigured))
	}
	if inviter == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	if inviter.Role != RoleOwner && inviter.Role != RoleAdmin {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthForbidden))
	}
	now := s.clock.Now().UTC()
	organisationID := id.MustParse(inviter.OrganisationID)

	if userRow, err := s.store.FindUserByEmail(ctx, emailAddress); err == nil {
		if user := userFromFindUserRow(userRow); user != nil {
			if _, membershipErr := s.store.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
				UserID: id.MustParse(user.ID),
				ID:     organisationID,
			}); membershipErr == nil {
				return nil, apperrors.Conflict(i18n.Msg(i18n.CodeAuthInvitationExists))
			} else if !errors.Is(membershipErr, pgx.ErrNoRows) {
				return nil, apperrors.Annotate(membershipErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindOrganisation))
			}
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindUser))
	}

	if pendingInvitation, err := s.store.FindPendingOrganisationInvitationByEmail(ctx, sqlcgen.FindPendingOrganisationInvitationByEmailParams{
		OrganisationID: organisationID,
		Email:          emailAddress,
	}); err == nil {
		if invitationFromFindRow(sqlcgen.FindOrganisationInvitationByTokenRow(pendingInvitation)) != nil {
			return nil, apperrors.Conflict(i18n.Msg(i18n.CodeAuthInvitationExists))
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindInvitation))
	}

	rawToken, err := security.RandomToken(32)
	if err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	invitationID := s.idg.New()
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
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageCreateInvitation))
	}
	if err := s.mailer.SendInvitation(ctx, email.InvitationEmail{
		To:               emailAddress,
		OrganisationName: inviter.OrganisationName,
		Role:             string(invitationRole),
		Token:            rawToken,
	}); err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageSendEmail))
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

func (s *Service) AcceptInvitation(ctx context.Context, token, displayName, password, ip, userAgent string) (string, error) {
	if s.beginTx == nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(errors.New("transaction opener is required for invitation acceptance")))
	}
	tx, err := s.beginTx(ctx)
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationBeginTx))
	}
	now := s.clock.Now().UTC()
	invitationRow, err := tx.queries.FindOrganisationInvitationByToken(ctx, security.HashToken(token))
	if err != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindInvitation)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvitationInvalid))
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindInvitation))
	}
	invitation := invitationFromFindRow(invitationRow)
	if invitation == nil || invitation.Status != InvitationStatusPending || invitation.AcceptedAtValid || now.After(invitation.ExpiresAt) {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvitationInvalid)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvitationInvalid))
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
				apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StagePasswordHash)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StagePasswordHash))
	}

	userRow, err := tx.queries.FindUserByEmail(ctx, emailAddress)
	var user *User
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindUser)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindUser))
	}
	if err == nil {
		user = userFromFindUserRow(userRow)
		if user == nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvitationInvalid)),
					apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
				)
			}
			return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvitationInvalid))
		}
		if !user.EmailVerified {
			if err := tx.queries.UpdateUserEmailVerifiedAt(ctx, sqlcgen.UpdateUserEmailVerifiedAtParams{
				ID:              id.MustParse(user.ID),
				EmailVerifiedAt: sqltype.Timestamptz(now),
				UpdatedAt:       sqltype.Timestamptz(now),
			}); err != nil {
				if rbErr := tx.rollback(); rbErr != nil {
					return "", errors.Join(
						apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageUpdateUserEmailVerifiedAt)),
						apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
					)
				}
				return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageUpdateUserEmailVerifiedAt))
			}
		}
	} else {
		userID := s.idg.New()
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
					apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation)),
					apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
				)
			}
			return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation))
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
					apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StagePasswordHash)),
					apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
				)
			}
			return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StagePasswordHash))
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.queries.InsertAccount(ctx, sqlcgen.InsertAccountParams{
			ID:                id.MustParse(s.idg.New()),
			UserID:            id.MustParse(user.ID),
			Provider:          string(AccountProviderPassword),
			ProviderAccountID: emailAddress,
			PasswordHash:      sqltype.Text(passwordHash),
			CreatedAt:         sqltype.Timestamptz(now),
			UpdatedAt:         sqltype.Timestamptz(now),
		}); err != nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation)),
					apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
				)
			}
			return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation))
		}
	} else {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindAccount)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindAccount))
	}

	if memberRow, err := tx.queries.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
		UserID: id.MustParse(user.ID),
		ID:     id.MustParse(invitation.OrganisationID),
	}); err == nil {
		_ = memberRow
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.queries.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{
			ID:             id.MustParse(s.idg.New()),
			OrganisationID: id.MustParse(invitation.OrganisationID),
			UserID:         id.MustParse(user.ID),
			Role:           string(invitation.Role),
			CreatedAt:      sqltype.Timestamptz(now),
			UpdatedAt:      sqltype.Timestamptz(now),
		}); err != nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageCreateInvitation)),
					apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
				)
			}
			return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageCreateInvitation))
		}
	} else {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindOrganisation)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindOrganisation))
	}

	if err := tx.queries.ConsumeOrganisationInvitation(ctx, sqlcgen.ConsumeOrganisationInvitationParams{
		ID:         id.MustParse(invitation.ID),
		AcceptedAt: sqltype.Timestamptz(now),
		UpdatedAt:  sqltype.Timestamptz(now),
	}); err != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageConsumeInvitation)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageConsumeInvitation))
	}
	if err := tx.commit(); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationCommit))
	}

	token, sessionErr := s.newSession(ctx, user.ID, invitation.OrganisationID, ip, userAgent)
	if sessionErr != nil {
		return "", sessionErr
	}
	return token, nil
}
