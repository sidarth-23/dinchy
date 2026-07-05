package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/eventbus"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

func (s *Service) Login(ctx context.Context, emailAddress, password, organisationSlug, totpCode, ip, userAgent string) (string, error) {
	user, err := s.findUserWithPassword(ctx, emailAddress, password)
	if err != nil {
		auditErr := s.publishEvent(ctx, eventbus.Event{
			Category:    "security",
			Subcategory: "auth",
			EventType:   string(events.AuthSecurityAuthLoginFailed),
			Action:      "login",
			Outcome:     "failed",
			IPAddress:   ip,
			UserAgent:   userAgent,
			Metadata:    events.AuthSecurityAuthLoginFailedMetadata{Email: emailAddress}.Map(),
		})
		if auditErr != nil {
			return "", errors.Join(err, auditErr)
		}
		return "", err
	}
	if err := s.verifyTOTPForLogin(ctx, user.ID, totpCode); err != nil {
		auditErr := s.publishEvent(ctx, eventbus.Event{
			Category:      "security",
			Subcategory:   "auth",
			EventType:     string(events.AuthSecurityAuthLoginFailed),
			Action:        "login",
			Outcome:       "failed",
			ActorUserID:   user.ID,
			TargetType:    "user",
			TargetID:      user.ID,
			TargetDisplay: user.Email,
			IPAddress:     ip,
			UserAgent:     userAgent,
			Metadata: events.AuthSecurityAuthLoginFailedMetadata{
				Email:  user.Email,
				Reason: "totp",
			}.Map(),
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
	if err := s.publishEvent(ctx, eventbus.Event{
		Category:            "security",
		Subcategory:         "auth",
		EventType:           string(events.AuthSecurityAuthLoginSucceeded),
		Action:              "login",
		Outcome:             "succeeded",
		ActorUserID:         user.ID,
		ActorOrganisationID: organisation.ID,
		TargetType:          "user",
		TargetID:            user.ID,
		TargetDisplay:       user.Email,
		IPAddress:           ip,
		UserAgent:           userAgent,
		Metadata: events.AuthSecurityAuthLoginSucceededMetadata{
			Email:            user.Email,
			OrganisationSlug: organisation.Slug,
		}.Map(),
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
	userID := id.MustParse(user.ID)
	accountRow, err := s.store.FindPasswordAccountByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
		}
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindAccount))
	}
	account := accountFromFindPasswordAccountRow(accountRow)
	if account == nil || !security.VerifyPassword(password, account.PasswordHash) {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	return user, nil
}

func userFromFindUserRow(row sqlcgen.FindUserByEmailRow) *User {
	if row.ID == uuid.Nil {
		return nil
	}
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName}
}

func accountFromFindPasswordAccountRow(row sqlcgen.FindPasswordAccountByUserIDRow) *Account {
	return &Account{
		ID:                row.ID.String(),
		UserID:            row.UserID.String(),
		Provider:          row.Provider,
		ProviderAccountID: row.ProviderAccountID,
		PasswordHash:      row.PasswordHash.String,
	}
}

func (s *Service) resolveLoginOrganisation(ctx context.Context, userID, slug string) (*Organisation, error) {
	slug = transform.Trim(slug)
	if slug != "" {
		orgRow, err := s.store.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{UserID: id.MustParse(userID), Slug: slug})
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
	orgRows, err := s.store.ListOrganisationsForUser(ctx, id.MustParse(userID))
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
