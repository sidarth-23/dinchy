package auth

import (
	"context"
	"errors"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/platform/events"
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
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCreateFirstUser), apperrors.WithCause(err))
	}
	now := s.Clock.Now()
	organizationID := s.IDGenerator.New()
	if s.beginTx == nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(errors.New("transaction opener is required for first-user setup")))
	}
	tx, err := s.beginTx(ctx)
	if err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupBeginTx), apperrors.WithCause(err))
	}
	user, err := createFirstUser(ctx, tx.queries, CreateUserInput{ID: s.IDGenerator.New(), AccountID: s.IDGenerator.New(), OrganizationID: organizationID, OrganizationMemberID: s.IDGenerator.New(), AdminRoleID: s.IDGenerator.New(), MemberRoleID: s.IDGenerator.New(), Email: emailAddress, PasswordHash: hash, DisplayName: displayName, OrganizationName: s.authConfig.DefaultOrganizationName, OrganizationSlug: s.authConfig.DefaultOrganizationSlug, Now: now})
	if err != nil {
		if rbErr := tx.rollback(); rbErr != nil {
			return "", errors.Join(apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCreateFirstUser), apperrors.WithCause(err)), apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupRollback), apperrors.WithCause(rbErr)))
		}
		return "", err
	}
	if err := tx.commit(); err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCommit), apperrors.WithCause(err))
	}
	envelope, err := events.NewEnvelope(ctx, user.ID, organizationID, events.NewTarget("user", user.ID, user.DisplayName))
	if err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCreateFirstUser), apperrors.WithCause(err))
	}
	if err := s.publishEvent(ctx, SecurityAuthSetupCompletedEvent{EventType: SecurityAuthSetupCompleted, Envelope: envelope, Metadata: NewSecurityAuthSetupCompletedMetadata(user.Email, user.DisplayName)}); err != nil {
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCreateFirstUser), apperrors.WithCause(err))
	}
	return s.sessions.Create(ctx, user.ID, organizationID, ip, userAgent)
}

func createFirstUser(ctx context.Context, q Store, in CreateUserInput) (User, error) {
	count, err := q.CountUsers(ctx)
	if err != nil {
		return User{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCountUsers), apperrors.WithCause(err))
	}
	if count > 0 {
		return User{}, apperrors.Conflict(i18n.Msg(i18n.CodeAccountAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", int(count))))
	}
	now := in.Now.UTC()
	if err := q.InsertUser(ctx, sqlcgen.InsertUserParams{ID: id.MustParse(in.ID), Email: in.Email, DisplayName: in.DisplayName, EmailVerifiedAt: sqltype.Timestamptz(now), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupInsertUser), apperrors.WithCause(err))
	}
	if err := q.InsertAccount(ctx, sqlcgen.InsertAccountParams{ID: id.MustParse(in.AccountID), UserID: id.MustParse(in.ID), Provider: string(AccountProviderPassword), ProviderAccountID: in.Email, PasswordHash: sqltype.Text(in.PasswordHash), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupInsertAccount), apperrors.WithCause(err))
	}
	if err := q.InsertOrganization(ctx, sqlcgen.InsertOrganizationParams{ID: id.MustParse(in.OrganizationID), Name: in.OrganizationName, Slug: in.OrganizationSlug, Logo: sqltype.Text(""), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupInsertOrganization), apperrors.WithCause(err))
	}
	roleIDs := map[permission.Role]string{permission.RoleAdmin: in.AdminRoleID, permission.RoleMember: in.MemberRoleID}
	for _, role := range permission.BuiltInRoles() {
		roleID := id.MustParse(roleIDs[role])
		if err := q.InsertOrganizationRole(ctx, sqlcgen.InsertOrganizationRoleParams{ID: roleID, OrganizationID: id.MustParse(in.OrganizationID), RoleKey: string(role), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
			return User{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCreateFirstUser), apperrors.WithCause(err))
		}
		for _, granted := range permission.DefaultRolePermissions(role) {
			if err := q.InsertOrganizationRolePermission(ctx, sqlcgen.InsertOrganizationRolePermissionParams{RoleID: roleID, Permission: string(granted)}); err != nil {
				return User{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupCreateFirstUser), apperrors.WithCause(err))
			}
		}
	}
	if err := q.InsertOrganizationMember(ctx, sqlcgen.InsertOrganizationMemberParams{ID: id.MustParse(in.OrganizationMemberID), OrganizationID: id.MustParse(in.OrganizationID), UserID: id.MustParse(in.ID), Role: string(permission.RoleAdmin), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return User{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSetupInsertOrganizationMember), apperrors.WithCause(err))
	}
	return User{ID: in.ID, Email: in.Email, DisplayName: in.DisplayName, EmailVerified: true}, nil
}
