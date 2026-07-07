// Package auth handles authentication, sessions, account setup, and related feature flows.
package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	cachecore "github.com/sidarth-23/dinchy/internal/platform/cache/core"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/eventbus"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

type Service struct {
	db         *pgxpool.Pool
	beginTx    func(context.Context) (*setupTransaction, error)
	store      Store
	idg        *id.Generator
	clock      clock.Clock
	authConfig config.AuthConfig
	sso        *ssoRegistry
	cache      cachecore.Store
	mailer     *email.Mailer
	publisher  eventbus.Publisher
}

func NewService(db *pgxpool.Pool, s Store, idg *id.Generator, clk clock.Clock, authConfig config.AuthConfig, providers []config.SSOProviderConfig, cacheStore cachecore.Store, cacheKeyer cachecore.Keyer, mailer *email.Mailer, publisher eventbus.Publisher) (*Service, error) {
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
	service := &Service{db: db, store: s, idg: idg, clock: clk, authConfig: authConfig, sso: registry, cache: cacheStore, mailer: mailer, publisher: publisher}
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

func organisationFromFindOrganisationRow(row sqlcgen.FindOrganisationBySlugForUserRow) *Organisation {
	organisation := organisationFromListOrganisationRow(sqlcgen.ListOrganisationsForUserRow{ID: row.ID, Name: row.Name, Slug: row.Slug, Role: row.Role})
	return &organisation
}

func organisationFromListOrganisationRow(row sqlcgen.ListOrganisationsForUserRow) Organisation {
	return Organisation{ID: row.ID.String(), Name: row.Name, Slug: row.Slug, Role: Role(row.Role)}
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

func (s *Service) Login(ctx context.Context, emailAddress, password, organisationSlug, totpCode, ip, userAgent string) (string, error) {
	user, err := s.findUserWithPassword(ctx, emailAddress, password)
	if err != nil {
		auditErr := s.publishEvent(ctx, events.AuthSecurityAuthLoginFailedEvent{
			EventType: events.AuthSecurityAuthLoginFailed,
			Envelope: events.Envelope{
				IPAddress: ip,
				UserAgent: userAgent,
			},
			Metadata: events.NewAuthSecurityAuthLoginFailedMetadata(emailAddress, ""),
		})
		if auditErr != nil {
			return "", errors.Join(err, auditErr)
		}
		return "", err
	}
	if err := s.verifyTOTPForLogin(ctx, user.ID, totpCode); err != nil {
		auditErr := s.publishEvent(ctx, events.AuthSecurityAuthLoginFailedEvent{
			EventType: events.AuthSecurityAuthLoginFailed,
			Envelope: events.Envelope{
				ActorUserID:   user.ID,
				TargetType:    "user",
				TargetID:      user.ID,
				TargetDisplay: user.Email,
				IPAddress:     ip,
				UserAgent:     userAgent,
			},
			Metadata: events.NewAuthSecurityAuthLoginFailedMetadata(user.Email, "totp"),
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
		Envelope: events.Envelope{
			ActorUserID:         user.ID,
			ActorOrganisationID: organisation.ID,
			TargetType:          "user",
			TargetID:            user.ID,
			TargetDisplay:       user.Email,
			IPAddress:           ip,
			UserAgent:           userAgent,
		},
		Metadata: events.NewAuthSecurityAuthLoginSucceededMetadata(user.Email, organisation.Slug),
	}); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageLogin))
	}
	return token, nil
}

func (s *Service) findUserWithPassword(ctx context.Context, emailAddress, password string) (*User, error) {
	row, err := s.store.FindUserByEmail(ctx, transform.Email(emailAddress))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
		if errors.Is(err, pgx.ErrNoRows) {
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
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName, EmailVerified: row.EmailVerifiedAt.Valid}
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
			if errors.Is(err, pgx.ErrNoRows) {
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

func (s *Service) newSession(ctx context.Context, userID, organisationID, ip, ua string) (string, error) {
	token, err := security.RandomToken(32)
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowNewSession),
			apperrors.WithStage(apperrors.StageGenerateToken),
		)
	}
	tokenHash := security.HashToken(token)
	now := s.clock.Now()
	err = s.store.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   id.MustParse(s.idg.New()),
		UserID:               id.MustParse(userID),
		ActiveOrganisationID: id.MustParse(organisationID),
		TokenHash:            tokenHash,
		IpAddress:            ip,
		UserAgent:            ua,
		LastSeenAt:           sqltype.Timestamptz(now),
		IdleExpiresAt:        sqltype.Timestamptz(now.Add(s.authConfig.SessionIdleTimeout)),
		ExpiresAt:            sqltype.Timestamptz(now.Add(s.authConfig.SessionMaxLifetime)),
		CreatedAt:            sqltype.Timestamptz(now),
		UpdatedAt:            sqltype.Timestamptz(now),
	})
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowNewSession),
			apperrors.WithStage(apperrors.StageCreateSession),
		)
	}
	return token, nil
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

func sessionFromGetSessionRow(row sqlcgen.GetSessionByTokenHashRow) *SessionWithUser {
	session := SessionWithUser{
		SessionID:        row.ID.String(),
		UserID:           row.UserID.String(),
		Email:            row.Email,
		DisplayName:      row.DisplayName,
		OrganisationID:   row.ActiveOrganisationID.String(),
		OrganisationName: row.OrganisationName,
		OrganisationSlug: row.OrganisationSlug,
		Role:             Role(row.Role),
		IdleExpiresAt:    sqltype.TimeValue(row.IdleExpiresAt),
		ExpiresAt:        sqltype.TimeValue(row.ExpiresAt),
	}
	if row.RevokedAt.Valid {
		session.RevokedAt = row.RevokedAt
	}
	return &session
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	session, sessionErr := s.Session(ctx, rawToken)
	now := s.clock.Now()
	err := s.store.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{RevokedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now), TokenHash: security.HashToken(rawToken)})
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageRevokeSession))
	}
	if sessionErr == nil && session != nil {
		if err := s.publishEvent(ctx, events.AuthSecurityAuthLogoutSucceededEvent{
			EventType: events.AuthSecurityAuthLogoutSucceeded,
			Envelope: events.Envelope{
				ActorUserID:         session.UserID,
				ActorOrganisationID: session.OrganisationID,
				TargetType:          "session",
				TargetID:            session.SessionID,
			},
			Metadata: events.NewAuthSecurityAuthLogoutSucceededMetadata(session.Email),
		}); err != nil {
			return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageLogout))
		}
	}
	return nil
}
