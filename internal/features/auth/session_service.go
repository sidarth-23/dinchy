package auth

import (
	"context"
	"database/sql"
	"errors"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/eventbus"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

func (s *Service) newSession(ctx context.Context, userID, organisationID, ip, ua string) (string, error) {
	token, tokenHash, err := generateSessionToken()
	if err != nil {
		return "", apperrors.Annotate(err,
			apperrors.WithFlow(apperrors.FlowNewSession),
			apperrors.WithStage(apperrors.StageGenerateToken),
		)
	}
	now := s.clock.Now()
	err = s.store.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   mustParseUUID(s.idg.New()),
		UserID:               mustParseUUID(userID),
		ActiveOrganisationID: mustParseUUID(organisationID),
		TokenHash:            tokenHash,
		IpAddress:            ip,
		UserAgent:            ua,
		LastSeenAt:           now.UTC(),
		IdleExpiresAt:        now.Add(s.authConfig.SessionIdleTimeout).UTC(),
		ExpiresAt:            now.Add(s.authConfig.SessionMaxLifetime).UTC(),
		CreatedAt:            now.UTC(),
		UpdatedAt:            now.UTC(),
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
	organisationRow, err := s.store.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{UserID: mustParseUUID(session.UserID), Slug: organisationSlug})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	row, err := s.store.GetSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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
	err := s.store.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{RevokedAt: sql.NullTime{Time: s.clock.Now().UTC(), Valid: true}, UpdatedAt: s.clock.Now().UTC(), TokenHash: hashToken(rawToken)})
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageRevokeSession))
	}
	if sessionErr == nil && session != nil {
		if err := s.publishEvent(ctx, eventbus.Event{
			Category:            "security",
			Subcategory:         "auth",
			EventType:           string(events.AuthSecurityAuthLogoutSucceeded),
			Action:              "logout",
			Outcome:             "succeeded",
			ActorUserID:         session.UserID,
			ActorOrganisationID: session.OrganisationID,
			TargetType:          "session",
			TargetID:            session.SessionID,
			Metadata:            events.AuthSecurityAuthLogoutSucceededMetadata{Email: session.Email}.Map(),
		}); err != nil {
			return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageLogout))
		}
	}
	return nil
}
