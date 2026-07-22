package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/module"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

// Service handles authentication, sessions, TOTP, invitations, and SSO for the auth feature.
type Service struct {
	*module.Service
	beginTx    func(context.Context) (*setupTransaction, error)
	store      Store
	sessions   *session.Service
	authConfig config.AuthConfig
	links      config.Links
	sso        *ssoRegistry
}

// NewService builds an auth Service, wiring the SSO registry and falling back to a no-op mailer when none is provided.
func NewService(base *module.Service, store Store, sessions *session.Service, authConfig config.AuthConfig, links config.Links, providers []config.SSOProviderConfig) (*Service, error) {
	if base == nil {
		return nil, apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(errors.New("auth module service is required")))
	}
	if err := base.Initialize(); err != nil {
		return nil, apperrors.Annotate(err)
	}
	registry, err := newSSORegistry(authConfig, providers, base.CacheKeyer)
	if err != nil {
		return nil, err
	}
	service := &Service{Service: base, store: store, sessions: sessions, authConfig: authConfig, links: links, sso: registry}
	if base.Database != nil {
		service.beginTx = func(ctx context.Context) (*setupTransaction, error) {
			tx, err := base.Database.Begin(ctx)
			if err != nil {
				return nil, err
			}
			return &setupTransaction{queries: sqlcgen.New(tx), commit: func() error { return tx.Commit(ctx) }, rollback: func() error { return tx.Rollback(ctx) }}, nil
		}
	}
	return service, nil
}

var _ module.Module = (*Service)(nil)

// Bootstrap reports whether first-user setup is still required and the current instance name.
func (s *Service) Bootstrap(ctx context.Context) (BootstrapState, error) {
	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	name, err := s.store.GetInstanceName(ctx)
	if err != nil {
		return BootstrapState{}, err
	}
	return BootstrapState{SetupRequired: count == 0, InstanceName: name}, nil
}

func (s *Service) publishEvent(ctx context.Context, event events.Event) error {
	if s.EventPublisher == nil {
		return nil
	}
	return s.EventPublisher.Publish(ctx, event)
}

// OrganizationsForUser lists the organizations the given user can access.
func (s *Service) OrganizationsForUser(ctx context.Context, userID string) ([]Organization, error) {
	rows, err := s.store.ListOrganizationsForUser(ctx, id.MustParse(userID))
	if err != nil {
		return nil, err
	}
	out := make([]Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, organizationFromListOrganizationRow(row))
	}
	return out, nil
}

// Login verifies credentials and TOTP, resolves the target organization, and returns a new session token.
func (s *Service) Login(ctx context.Context, emailAddress, password, organizationSlug, totpCode, ip, userAgent string) (string, error) {
	user, err := s.findUserWithPassword(ctx, emailAddress, password)
	if err != nil {
		return "", err
	}
	if err := s.verifyTOTPForLogin(ctx, user.ID, totpCode); err != nil {
		return "", err
	}
	organization, err := s.resolveLoginOrganization(ctx, user.ID, organizationSlug)
	if err != nil {
		return "", err
	}
	return s.sessions.Create(ctx, user.ID, organization.ID, ip, userAgent)
}

// SelectOrganization switches the current session to another organization the user belongs to, returning a fresh session token.
func (s *Service) SelectOrganization(ctx context.Context, rawToken, organizationSlug, ip, userAgent string) (string, error) {
	principal, err := s.sessions.Session(ctx, rawToken)
	if err != nil {
		return "", err
	}
	if principal == nil {
		return "", apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthUnauthenticated))
	}
	organizationRow, err := s.store.FindOrganizationBySlugForUser(ctx, sqlcgen.FindOrganizationBySlugForUserParams{UserID: id.MustParse(principal.UserID), Slug: organizationSlug})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthOrganizationNotFound))
		}
		return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthSessionFindOrganization), apperrors.WithCause(err))
	}
	organization := organizationFromFindOrganizationRow(organizationRow)
	if organization == nil {
		return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthOrganizationNotFound))
	}
	loggedOutPrincipal, err := s.sessions.Logout(ctx, rawToken)
	if err != nil {
		return "", err
	}
	if loggedOutPrincipal != nil {
		envelope, err := events.NewEnvelope(ctx, loggedOutPrincipal.UserID, loggedOutPrincipal.OrganizationID, events.NewTarget("session", loggedOutPrincipal.SessionID, loggedOutPrincipal.DisplayName))
		if err != nil {
			return "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLogoutPublishEvent), apperrors.WithCause(err))
		}
		// Best effort: logout should not fail if audit publication fails.
		_ = s.publishEvent(ctx, events.AuthSecurityAuthLogoutSucceededEvent{EventType: events.AuthSecurityAuthLogoutSucceeded, Envelope: envelope, Metadata: events.NewAuthSecurityAuthLogoutSucceededMetadata(loggedOutPrincipal.Email)})
	}
	return s.sessions.Create(ctx, principal.UserID, organization.ID, ip, userAgent)
}
