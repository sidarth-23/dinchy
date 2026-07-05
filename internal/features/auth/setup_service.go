package auth

import (
	"context"
	"database/sql"
	"errors"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/eventbus"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/sqlutil"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

type setupTransaction struct {
	queries  Store
	commit   func() error
	rollback func() error
}

func (s *Service) SetupFirstUser(ctx context.Context, emailAddress, displayName, password, ip, userAgent string) (string, error) {
	hash, err := security.HashPassword(password)
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser))
	}
	emailAddress = transform.Email(emailAddress)
	now := s.clock.Now()
	organisationID := s.idg.New()
	if s.beginTx == nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(errors.New("transaction opener is required for first-user setup")))
	}
	tx, err := s.beginTx(ctx)
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationBeginTx))
	}
	user, err := createFirstUser(ctx, tx.queries, CreateUserInput{
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
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(
				apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser)),
				apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationRollback)),
			)
		}
		return "", err
	}
	if err := tx.commit(); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationCommit))
	}
	if err := s.publishEvent(ctx, eventbus.Event{
		Category:            "security",
		Subcategory:         "auth",
		EventType:           string(events.AuthSecurityAuthSetupCompleted),
		Action:              "setup_first_user",
		Outcome:             "succeeded",
		ActorUserID:         user.ID,
		ActorOrganisationID: organisationID,
		TargetType:          "user",
		TargetID:            user.ID,
		TargetDisplay:       user.Email,
		IPAddress:           ip,
		UserAgent:           userAgent,
		Metadata:            events.AuthSecurityAuthSetupCompletedMetadata{Email: user.Email, DisplayName: user.DisplayName}.Map(),
	}); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser))
	}
	return s.newSession(ctx, user.ID, organisationID, ip, userAgent)
}

func createFirstUser(ctx context.Context, q Store, in CreateUserInput) (User, error) {
	count, err := q.CountUsers(ctx)
	if err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationCountUsers))
	}
	if count > 0 {
		return User{}, apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", int(count))))
	}
	now := in.Now.UTC()
	if err := q.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:          id.MustParse(in.ID),
		Email:       in.Email,
		DisplayName: in.DisplayName,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertUser))
	}
	if err := q.InsertAccount(ctx, sqlcgen.InsertAccountParams{
		ID:                id.MustParse(in.AccountID),
		UserID:            id.MustParse(in.ID),
		Provider:          string(AccountProviderPassword),
		ProviderAccountID: in.Email,
		PasswordHash:      sqlutil.NullString(in.PasswordHash),
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertAccount))
	}
	if err := q.InsertOrganisation(ctx, sqlcgen.InsertOrganisationParams{
		ID:        id.MustParse(in.OrganisationID),
		Name:      in.OrganisationName,
		Slug:      in.OrganisationSlug,
		Logo:      sql.NullString{},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertOrganisation))
	}
	if err := q.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{
		ID:             id.MustParse(in.OrganisationMemberID),
		OrganisationID: id.MustParse(in.OrganisationID),
		UserID:         id.MustParse(in.ID),
		Role:           string(RoleOwner),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertOrganisationMember))
	}
	return User{ID: in.ID, Email: in.Email, DisplayName: in.DisplayName}, nil
}
