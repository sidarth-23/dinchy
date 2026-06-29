package auth

import (
	"context"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

func (s *Service) Login(ctx context.Context, emailAddress, password, organisationSlug, totpCode, ip, userAgent string) (string, error) {
	user, err := s.findUserWithPassword(ctx, emailAddress, password)
	if err != nil {
		return "", err
	}
	if err := s.verifyTOTPForLogin(ctx, user.ID, totpCode); err != nil {
		return "", err
	}
	organisation, err := s.resolveLoginOrganisation(ctx, user.ID, organisationSlug)
	if err != nil {
		return "", err
	}
	return s.newSession(ctx, user.ID, organisation.ID, ip, userAgent)
}

func (s *Service) findUserWithPassword(ctx context.Context, emailAddress, password string) (*User, error) {
	user, err := s.store.FindUserByEmail(ctx, transform.Email(emailAddress))
	if err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindUser))
	}
	if user == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	account, err := s.store.FindPasswordAccountByUserID(ctx, user.ID)
	if err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindAccount))
	}
	if account == nil || !verifyPassword(password, account.PasswordHash) {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	return user, nil
}

func (s *Service) resolveLoginOrganisation(ctx context.Context, userID, slug string) (*Organisation, error) {
	slug = transform.Trim(slug)
	if slug != "" {
		org, err := s.store.FindOrganisationBySlugForUser(ctx, userID, slug)
		if err != nil {
			return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindOrganisation))
		}
		if org == nil {
			return nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
		}
		return org, nil
	}
	orgs, err := s.store.ListOrganisationsForUser(ctx, userID)
	if err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageListOrganisations))
	}
	if len(orgs) == 0 {
		return nil, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
	}
	if len(orgs) > 1 {
		return nil, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationRequired))
	}
	return &orgs[0], nil
}
