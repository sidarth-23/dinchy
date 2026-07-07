package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/redis"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

type Service struct {
	db         *pgxpool.Pool
	beginTx    func(context.Context) (*setupTransaction, error)
	store      Store
	idg        *id.Generator
	clock      clock.Clock
	authConfig config.AuthConfig
	sso        *ssoRegistry
	redis      *goredis.Client
	mailer     *email.Mailer
	publisher  events.Publisher
}

func NewService(db *pgxpool.Pool, s Store, idg *id.Generator, clk clock.Clock, authConfig config.AuthConfig, providers []config.SSOProviderConfig, redisClient *goredis.Client, cacheKeyer redis.Keyer, mailer *email.Mailer, publisher events.Publisher) (*Service, error) {
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
	service := &Service{
		db:         db,
		store:      s,
		idg:        idg,
		clock:      clk,
		authConfig: authConfig,
		sso:        registry,
		redis:      redisClient,
		mailer:     mailer,
		publisher:  publisher,
	}
	if db != nil {
		service.beginTx = func(ctx context.Context) (*setupTransaction, error) {
			tx, err := db.Begin(ctx)
			if err != nil {
				return nil, err
			}
			return &setupTransaction{
				queries:  sqlcgen.New(tx),
				commit:   func() error { return tx.Commit(ctx) },
				rollback: func() error { return tx.Rollback(ctx) },
			}, nil
		}
	}
	return service, nil
}

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

func (s *Service) OrganisationsForUser(ctx context.Context, userID string) ([]Organisation, error) {
	rows, err := s.store.ListOrganisationsForUser(ctx, id.MustParse(userID))
	if err != nil {
		return nil, err
	}
	out := make([]Organisation, 0, len(rows))
	for _, row := range rows {
		out = append(out, organisationFromListOrganisationRow(row))
	}
	return out, nil
}

func (s *Service) Login(ctx context.Context, emailAddress, password, organisationSlug, totpCode, ip, userAgent string) (string, error) {
	user, err := s.findUserWithPassword(ctx, emailAddress, password)
	if err != nil {
		auditErr := s.publishEvent(ctx, events.AuthSecurityAuthLoginFailedEvent{
			EventType: events.AuthSecurityAuthLoginFailed,
			Envelope:  events.Envelope{IPAddress: ip, UserAgent: userAgent},
			Metadata:  events.NewAuthSecurityAuthLoginFailedMetadata(emailAddress, ""),
		})
		if auditErr != nil {
			return "", errors.Join(err, auditErr)
		}
		return "", err
	}
	if err := s.verifyTOTPForLogin(ctx, user.ID, totpCode); err != nil {
		auditErr := s.publishEvent(ctx, events.AuthSecurityAuthLoginFailedEvent{
			EventType: events.AuthSecurityAuthLoginFailed,
			Envelope:  events.Envelope{ActorUserID: user.ID, TargetType: "user", TargetID: user.ID, TargetDisplay: user.Email, IPAddress: ip, UserAgent: userAgent},
			Metadata:  events.NewAuthSecurityAuthLoginFailedMetadata(user.Email, "totp"),
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
	if err := s.publishEvent(ctx, events.AuthSecurityAuthLoginSucceededEvent{
		EventType: events.AuthSecurityAuthLoginSucceeded,
		Envelope:  events.Envelope{ActorUserID: user.ID, ActorOrganisationID: organisation.ID, TargetType: "user", TargetID: user.ID, TargetDisplay: user.Email, IPAddress: ip, UserAgent: userAgent},
		Metadata:  events.NewAuthSecurityAuthLoginSucceededMetadata(user.Email, organisation.Slug),
	}); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageLogin))
	}
	return token, nil
}

func (s *Service) Session(ctx context.Context, rawToken string) (*SessionWithUser, error) {
	if rawToken == "" {
		return nil, nil
	}
	row, err := s.store.GetSessionByTokenHash(ctx, security.HashToken(rawToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSession), apperrors.WithStage(apperrors.StageGetSession))
	}
	session := sessionFromGetSessionRow(row)
	now := s.clock.Now()
	if session.RevokedAt.Valid || now.After(session.IdleExpiresAt) || now.After(session.ExpiresAt) {
		return nil, nil
	}
	return session, nil
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	session, sessionErr := s.Session(ctx, rawToken)
	now := s.clock.Now()
	if err := s.store.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{RevokedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now), TokenHash: security.HashToken(rawToken)}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageRevokeSession))
	}
	if sessionErr == nil && session != nil {
		if err := s.publishEvent(ctx, events.AuthSecurityAuthLogoutSucceededEvent{EventType: events.AuthSecurityAuthLogoutSucceeded, Envelope: events.Envelope{ActorUserID: session.UserID, ActorOrganisationID: session.OrganisationID, TargetType: "session", TargetID: session.SessionID}, Metadata: events.NewAuthSecurityAuthLogoutSucceededMetadata(session.Email)}); err != nil {
			return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageLogout))
		}
	}
	return nil
}

func (s *Service) SelectOrganisation(ctx context.Context, rawToken, organisationSlug, ip, userAgent string) (string, error) {
	session, err := s.Session(ctx, rawToken)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	organisationRow, err := s.store.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{UserID: id.MustParse(session.UserID), Slug: organisationSlug})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
		}
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSession), apperrors.WithStage(apperrors.StageFindOrganisation))
	}
	organisation := organisationFromFindOrganisationRow(organisationRow)
	if organisation == nil {
		return "", apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
	}
	if err := s.Logout(ctx, rawToken); err != nil {
		return "", err
	}
	return s.newSession(ctx, session.UserID, organisation.ID, ip, userAgent)
}
