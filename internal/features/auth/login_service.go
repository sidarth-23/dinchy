package auth

import (
	"context"
	"errors"
	"database/sql"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
)

func (s *Service) Login(ctx context.Context, emailAddress, password, organisationSlug, totpCode, ip, userAgent string) (string, error) {
	user, err := s.findUserWithPassword(ctx, emailAddress, password)
	if err != nil {
		auditErr := s.recordAudit(ctx, AuditEvent{
			Category:    "security",
			Subcategory: "auth",
			EventType:   "auth.login_failed",
			Action:      "login",
			Outcome:     "failed",
			IPAddress:   ip,
			UserAgent:   userAgent,
			Metadata:    map[string]any{"email": emailAddress},
		})
		if auditErr != nil {
			return "", errors.Join(err, auditErr)
		}
		return "", err
	}
	if err := s.verifyTOTPForLogin(ctx, user.ID, totpCode); err != nil {
		auditErr := s.recordAudit(ctx, AuditEvent{
			Category:      "security",
			Subcategory:   "auth",
			EventType:     "auth.login_failed",
			Action:        "login",
			Outcome:       "failed",
			ActorUserID:   user.ID,
			TargetType:    "user",
			TargetID:      user.ID,
			TargetDisplay: user.Email,
			IPAddress:     ip,
			UserAgent:     userAgent,
			Metadata:      map[string]any{"email": user.Email, "reason": "totp"},
		})
		if auditErr != nil {
			return "", errors.Join(err, auditErr)
		}
		return "", err
	}
	organisation, err := s.resolveLoginOrganisation(ctx, user.ID, organisationSlug)
	if err != nil {
		return "", err
	}
	token, err := s.newSession(ctx, user.ID, organisation.ID, ip, userAgent)
	if err != nil {
		return "", err
	}
	if err := s.recordAudit(ctx, AuditEvent{
		Category:            "security",
		Subcategory:         "auth",
		EventType:           "auth.login_succeeded",
		Action:              "login",
		Outcome:             "succeeded",
		ActorUserID:         user.ID,
		ActorOrganisationID: organisation.ID,
		TargetType:          "user",
		TargetID:            user.ID,
		TargetDisplay:       user.Email,
		IPAddress:           ip,
		UserAgent:           userAgent,
		Metadata:            map[string]any{"email": user.Email, "organisation_slug": organisation.Slug},
	}); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageLogin))
	}
	return token, nil
}

func (s *Service) findUserWithPassword(ctx context.Context, emailAddress, password string) (*User, error) {
	row, err := s.store.FindUserByEmail(ctx, transform.Email(emailAddress))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
		}
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
	}
	user := userFromFindUserRow(row)
	if user == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	userID := mustParseUUID(user.ID)
	accountRow, err := s.store.FindPasswordAccountByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
		}
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindAccount))
	}
	account := accountFromFindPasswordAccountRow(accountRow)
	if account == nil || !verifyPassword(password, account.PasswordHash) {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	return user, nil
}

func (s *Service) resolveLoginOrganisation(ctx context.Context, userID, slug string) (*Organisation, error) {
	slug = transform.Trim(slug)
	if slug != "" {
		orgRow, err := s.store.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{UserID: mustParseUUID(userID), Slug: slug})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
			}
			return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindOrganisation))
		}
		org := organisationFromFindOrganisationRow(orgRow)
		if org == nil {
			return nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
		}
		return org, nil
	}
	orgRows, err := s.store.ListOrganisationsForUser(ctx, mustParseUUID(userID))
	if err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageListOrganisations))
	}
	if len(orgRows) == 0 {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
	}
	if len(orgRows) > 1 {
		return nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationRequired))
	}
	org := organisationFromListOrganisationRow(orgRows[0])
	return &org, nil
}
