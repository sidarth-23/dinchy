package auth

import (
	"context"
	"errors"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

type setupTransaction struct {
	queries  Store
	commit   func() error
	rollback func() error
}

// SetupFirstUser creates the initial owner, account, and organization in one transaction and returns a session token.
func (s *Service) SetupFirstUser(ctx context.Context, emailAddress, displayName, password, ip, userAgent string) (string, error) {
	hash, err := security.HashPassword(password)
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser))
	}
	now := s.clock.Now()
	organisationID := s.idg.New()
	if s.beginTx == nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(errors.New("transaction opener is required for first-user setup")))
	}
	tx, err := s.beginTx(ctx)
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationBeginTx))
	}
	user, err := createFirstUser(ctx, tx.queries, CreateUserInput{ID: s.idg.New(), AccountID: s.idg.New(), OrganisationID: organisationID, OrganisationMemberID: s.idg.New(), AdminRoleID: s.idg.New(), MemberRoleID: s.idg.New(), Email: emailAddress, PasswordHash: hash, DisplayName: displayName, OrganisationName: s.authConfig.DefaultOrganisationName, OrganisationSlug: s.authConfig.DefaultOrganisationSlug, Now: now})
	if err != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser)), apperrors.Annotate(rbErr, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationRollback)))
		}
		return "", err
	}
	if err := tx.commit(); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationCommit))
	}
	if err := s.publishEvent(ctx, events.AuthSecurityAuthSetupCompletedEvent{EventType: events.AuthSecurityAuthSetupCompleted, Envelope: events.Envelope{ActorUserID: user.ID, ActorOrganisationID: organisationID, TargetType: "user", TargetID: user.ID, TargetDisplay: user.Email, IPAddress: ip, UserAgent: userAgent}, Metadata: events.NewAuthSecurityAuthSetupCompletedMetadata(user.Email, user.DisplayName)}); err != nil {
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
	if err := q.InsertUser(ctx, sqlcgen.InsertUserParams{ID: id.MustParse(in.ID), Email: in.Email, DisplayName: in.DisplayName, EmailVerifiedAt: sqltype.Timestamptz(now), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertUser))
	}
	if err := q.InsertAccount(ctx, sqlcgen.InsertAccountParams{ID: id.MustParse(in.AccountID), UserID: id.MustParse(in.ID), Provider: string(AccountProviderPassword), ProviderAccountID: in.Email, PasswordHash: sqltype.Text(in.PasswordHash), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertAccount))
	}
	if err := q.InsertOrganisation(ctx, sqlcgen.InsertOrganisationParams{ID: id.MustParse(in.OrganisationID), Name: in.OrganisationName, Slug: in.OrganisationSlug, Logo: sqltype.Text(""), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertOrganisation))
	}
	roleIDs := map[permission.Role]string{permission.RoleAdmin: in.AdminRoleID, permission.RoleMember: in.MemberRoleID}
	for _, role := range permission.BuiltInRoles() {
		roleID := id.MustParse(roleIDs[role])
		if err := q.InsertOrganisationRole(ctx, sqlcgen.InsertOrganisationRoleParams{ID: roleID, OrganisationID: id.MustParse(in.OrganisationID), RoleKey: string(role), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
			return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser))
		}
		for _, granted := range permission.DefaultRolePermissions(role) {
			if err := q.InsertOrganisationRolePermission(ctx, sqlcgen.InsertOrganisationRolePermissionParams{RoleID: roleID, Permission: string(granted)}); err != nil {
				return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser))
			}
		}
	}
	if err := q.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{ID: id.MustParse(in.OrganisationMemberID), OrganisationID: id.MustParse(in.OrganisationID), UserID: id.MustParse(in.ID), Role: string(permission.RoleAdmin), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSetupFirstUser), apperrors.WithStage(apperrors.StageSetupFirstUser), apperrors.WithOperation(apperrors.OperationInsertOrganisationMember))
	}
	return User{ID: in.ID, Email: in.Email, DisplayName: in.DisplayName, EmailVerified: true}, nil
}
