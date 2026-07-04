package auth

import (
	"context"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

func (s *Service) SelectOrganisation(ctx context.Context, rawToken, organisationSlug, ip, userAgent string) (string, error) {
	session, err := s.Session(ctx, rawToken)
	if err != nil {
		return "", err
	}
	if session == nil {
		return "", apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthUnauthenticated))
	}
	organisation, err := s.store.FindOrganisationBySlugForUser(ctx, session.UserID, organisationSlug)
	if err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSession), apperrors.WithStage(apperrors.StageFindOrganisation))
	}
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
	session, err := s.store.GetSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil || session == nil {
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowSession), apperrors.WithStage(apperrors.StageGetSession))
	}
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
	var session *SessionWithUser
	var sessionErr error
	if s.audit != nil {
		session, sessionErr = s.Session(ctx, rawToken)
	}
	err := s.store.RevokeSessionByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageRevokeSession))
	}
	if sessionErr == nil && session != nil {
		if err := s.recordAudit(ctx, AuditEvent{
			Category:            "security",
			Subcategory:         "auth",
			EventType:           "auth.logout_succeeded",
			Action:              "logout",
			Outcome:             "succeeded",
			ActorUserID:         session.UserID,
			ActorOrganisationID: session.OrganisationID,
			TargetType:          "session",
			TargetID:            session.SessionID,
			Metadata:            map[string]any{"email": session.Email},
		}); err != nil {
			return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogout), apperrors.WithStage(apperrors.StageLogout))
		}
	}
	return nil
}
