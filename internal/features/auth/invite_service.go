package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/sqlutil"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

const (
	invitationRoleMember    = "member"
	invitationRoleAdmin     = "admin"
	invitationStatusPending = "pending"
)

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
		ExpiresAt:       row.ExpiresAt.UTC(),
		InvitedByUserID: row.InvitedByUserID.String(),
	}
	if row.AcceptedAt.Valid {
		invitation.AcceptedAt = row.AcceptedAt.Time.UTC()
		invitation.AcceptedAtValid = true
	}
	return invitation
}

func invitationFromPendingRow(row sqlcgen.FindPendingOrganisationInvitationByEmailRow) *Invitation {
	return invitationFromFindRow(sqlcgen.FindOrganisationInvitationByTokenRow(row))
}

func parseInvitationRole(role string) (Role, error) {
	role = strings.ToLower(transform.Trim(role))
	switch role {
	case invitationRoleMember, invitationRoleAdmin:
		return Role(role), nil
	default:
		return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvitationRoleInvalid))
	}
}

func (s *Service) CreateInvitation(ctx context.Context, inviter *SessionWithUser, emailAddress, role, ip, userAgent string) (*Invitation, error) {
	if !s.email.Configured() {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeEmailNotConfigured), apperrors.WithCause(email.ErrNotConfigured))
	}
	if inviter == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	if inviter.Role != RoleOwner && inviter.Role != RoleAdmin {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthForbidden))
	}
	invitationRole, err := parseInvitationRole(role)
	if err != nil {
		return nil, err
	}
	emailAddress = transform.Email(emailAddress)
	now := s.clock.Now().UTC()
	organisationID := id.MustParse(inviter.OrganisationID)

	if userRow, err := s.store.FindUserByEmail(ctx, emailAddress); err == nil {
		if user := userFromFindUserRow(userRow); user != nil {
			if _, membershipErr := s.store.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
				UserID: id.MustParse(user.ID),
				ID:     organisationID,
			}); membershipErr == nil {
				return nil, apperrors.Conflict(i18n.Msg(i18n.CodeAuthInvitationExists))
			} else if !errors.Is(membershipErr, sql.ErrNoRows) {
				return nil, apperrors.Annotate(membershipErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindOrganisation))
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageFindUser))
	}

	if pendingInvitation, err := s.store.FindPendingOrganisationInvitationByEmail(ctx, sqlcgen.FindPendingOrganisationInvitationByEmailParams{
		OrganisationID: organisationID,
		Email:          emailAddress,
	}); err == nil {
		if invitationFromPendingRow(pendingInvitation) != nil {
			return nil, apperrors.Conflict(i18n.Msg(i18n.CodeAuthInvitationExists))
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
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
		ExpiresAt:       expiresAt,
		InvitedByUserID: id.MustParse(inviter.UserID),
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageCreateInvitation))
	}
	if err := s.email.Send(ctx, email.Message{
		To:      emailAddress,
		Subject: "You are invited to Dinchy",
		Text:    invitationEmailText(inviter.OrganisationName, string(invitationRole), rawToken),
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
		if errors.Is(err, sql.ErrNoRows) {
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
	userDisplayName := transform.Trim(displayName)
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
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
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
				EmailVerifiedAt: sql.NullTime{Time: now, Valid: true},
				UpdatedAt:       now,
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
			EmailVerifiedAt: sql.NullTime{Time: now, Valid: true},
			CreatedAt:       now,
			UpdatedAt:       now,
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
			PasswordHash: sqlutil.NullString(passwordHash),
			UpdatedAt:    now,
		}); err != nil {
			if rbErr := tx.rollback(); rbErr != nil {
				return "", errors.Join(
					apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StagePasswordHash)),
					apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StageAcceptInvitation), apperrors.WithOperation(apperrors.OperationRollback)),
				)
			}
			return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowInvitation), apperrors.WithStage(apperrors.StagePasswordHash))
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		if err := tx.queries.InsertAccount(ctx, sqlcgen.InsertAccountParams{
			ID:                id.MustParse(s.idg.New()),
			UserID:            id.MustParse(user.ID),
			Provider:          string(AccountProviderPassword),
			ProviderAccountID: emailAddress,
			PasswordHash:      sqlutil.NullString(passwordHash),
			CreatedAt:         now,
			UpdatedAt:         now,
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
	} else if errors.Is(err, sql.ErrNoRows) {
		if err := tx.queries.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{
			ID:             id.MustParse(s.idg.New()),
			OrganisationID: id.MustParse(invitation.OrganisationID),
			UserID:         id.MustParse(user.ID),
			Role:           string(invitation.Role),
			CreatedAt:      now,
			UpdatedAt:      now,
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
		AcceptedAt: now,
		UpdatedAt:  now,
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
