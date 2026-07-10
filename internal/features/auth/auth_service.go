package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/access/session"
	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	platformredis "github.com/sidarth-23/dinchy/internal/platform/redis"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

// Service handles authentication, sessions, TOTP, invitations, and SSO for the auth feature.
type Service struct {
	db         *pgxpool.Pool
	beginTx    func(context.Context) (*setupTransaction, error)
	store      Store
	sessions   *session.Service
	idg        *id.Generator
	clock      clock.Clock
	authConfig config.AuthConfig
	sso        *ssoRegistry
	redis      *goredis.Client
	mailer     *email.Mailer
	publisher  events.Publisher
}

// NewService builds an auth Service, wiring the SSO registry and falling back to a no-op mailer when none is provided.
func NewService(db *pgxpool.Pool, s Store, sessionSvc *session.Service, idg *id.Generator, clk clock.Clock, authConfig config.AuthConfig, providers []config.SSOProviderConfig, redisClient *goredis.Client, cacheKeyer platformredis.Keyer, mailer *email.Mailer, publisher events.Publisher) (*Service, error) {
	registry, err := newSSORegistry(authConfig, providers, cacheKeyer)
	if err != nil {
		return nil, err
	}
	if mailer == nil {
		mailer, err = email.NewMailer(email.NoopSender{}, "")
		if err != nil {
			return nil, err
		}
	}
	service := &Service{db: db, store: s, sessions: sessionSvc, idg: idg, clock: clk, authConfig: authConfig, sso: registry, redis: redisClient, mailer: mailer, publisher: publisher}
	if db != nil {
		service.beginTx = func(ctx context.Context) (*setupTransaction, error) {
			tx, err := db.Begin(ctx)
			if err != nil {
				return nil, err
			}
			return &setupTransaction{queries: sqlcgen.New(tx), commit: func() error { return tx.Commit(ctx) }, rollback: func() error { return tx.Rollback(ctx) }}, nil
		}
	}
	return service, nil
}

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
	if s.publisher == nil {
		return nil
	}
	return s.publisher.Publish(ctx, event)
}

// OrganisationsForUser lists the organizations the given user can access.
func (s *Service) OrganisationsForUser(ctx context.Context, userID string) ([]Organization, error) {
	rows, err := s.store.ListOrganisationsForUser(ctx, id.MustParse(userID))
	if err != nil {
		return nil, err
	}
	out := make([]Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, organisationFromListOrganisationRow(row))
	}
	return out, nil
}

// Login verifies credentials and TOTP, resolves the target organization, and returns a new session token.
func (s *Service) Login(ctx context.Context, emailAddress, password, organisationSlug, totpCode, ip, userAgent string) (string, error) {
	user, err := s.findUserWithPassword(ctx, emailAddress, password)
	if err != nil {
		return "", err
	}
	if err := s.verifyTOTPForLogin(ctx, user.ID, totpCode); err != nil {
		return "", err
	}
	organization, err := s.resolveLoginOrganisation(ctx, user.ID, organisationSlug)
	if err != nil {
		return "", err
	}
	return s.sessions.Create(ctx, user.ID, organization.ID, ip, userAgent)
}

// SelectOrganisation switches the current session to another organization the user belongs to, returning a fresh session token.
func (s *Service) SelectOrganisation(ctx context.Context, rawToken, organisationSlug, ip, userAgent string) (string, error) {
	principal, err := s.sessions.Session(ctx, rawToken)
	if err != nil {
		return "", err
	}
	if principal == nil {
		return "", apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	organisationRow, err := s.store.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{UserID: id.MustParse(principal.UserID), Slug: organisationSlug})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSession), apperrors.WithStage(apperrors.StageFindOrganisation))
	}
	organization := organisationFromFindOrganisationRow(organisationRow)
	if organization == nil {
		return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
	}
	loggedOutPrincipal, err := s.sessions.Logout(ctx, rawToken)
	if err != nil {
		return "", err
	}
	if loggedOutPrincipal != nil {
		_ = s.publishEvent(ctx, events.AuthSecurityAuthLogoutSucceededEvent{EventType: events.AuthSecurityAuthLogoutSucceeded, Envelope: events.Envelope{ActorUserID: loggedOutPrincipal.UserID, ActorOrganisationID: loggedOutPrincipal.OrganisationID, TargetType: "session", TargetID: loggedOutPrincipal.SessionID}, Metadata: events.NewAuthSecurityAuthLogoutSucceededMetadata(loggedOutPrincipal.Email)})
	}
	return s.sessions.Create(ctx, principal.UserID, organization.ID, ip, userAgent)
}
