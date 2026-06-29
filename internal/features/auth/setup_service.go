package auth

import (
	"context"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

func (s *Service) SetupFirstUser(ctx context.Context, emailAddress, displayName, password, ip, userAgent string) (string, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser))
	}
	emailAddress = transform.Email(emailAddress)
	now := s.clock.Now()
	organisationID := s.idg.New()
	user, err := s.store.CreateFirstUser(ctx, CreateUserInput{
		ID:                   s.idg.New(),
		AccountID:            s.idg.New(),
		OrganisationID:       organisationID,
		OrganisationMemberID: s.idg.New(),
		Email:                emailAddress,
		PasswordHash:         hash,
		DisplayName:          transform.Trim(displayName),
		OrganisationName:     s.authConfig.DefaultOrganisationName,
		OrganisationSlug:     s.authConfig.DefaultOrganisationSlug,
		Now:                  now,
	})
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageCreateFirstUser))
	}
	return s.newSession(ctx, user.ID, organisationID, ip, userAgent)
}
