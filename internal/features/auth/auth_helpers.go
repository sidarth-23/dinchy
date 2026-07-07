package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

func userFromFindUserRow(row sqlcgen.FindUserByEmailRow) *User {
	if row.ID == uuid.Nil {
		return nil
	}
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName, EmailVerified: row.EmailVerifiedAt.Valid}
}
func organisationFromListOrganisationRow(row sqlcgen.ListOrganisationsForUserRow) Organisation {
	return Organisation{ID: row.ID.String(), Name: row.Name, Slug: row.Slug, Role: Role(row.Role)}
}
func organisationFromFindOrganisationRow(row sqlcgen.FindOrganisationBySlugForUserRow) *Organisation {
	if row.ID == uuid.Nil {
		return nil
	}
	org := organisationFromListOrganisationRow(sqlcgen.ListOrganisationsForUserRow{ID: row.ID, Name: row.Name, Slug: row.Slug, Role: row.Role})
	return &org
}
func sessionFromGetSessionRow(row sqlcgen.GetSessionByTokenHashRow) *SessionWithUser {
	s := SessionWithUser{SessionID: row.ID.String(), UserID: row.UserID.String(), Email: row.Email, DisplayName: row.DisplayName, OrganisationID: row.ActiveOrganisationID.String(), OrganisationName: row.OrganisationName, OrganisationSlug: row.OrganisationSlug, Role: Role(row.Role), IdleExpiresAt: sqltype.TimeValue(row.IdleExpiresAt), ExpiresAt: sqltype.TimeValue(row.ExpiresAt), RevokedAt: row.RevokedAt}
	return &s
}

func (s *Service) findUserWithPassword(ctx context.Context, emailAddress, password string) (*User, error) {
	row, err := s.store.FindUserByEmail(ctx, emailAddress)
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
	accountRow, err := s.store.FindPasswordAccountByUserID(ctx, id.MustParse(user.ID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
		}
		return nil, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindAccount))
	}
	if !security.VerifyPassword(password, accountRow.PasswordHash.String) {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidCredentials))
	}
	return user, nil
}
func (s *Service) resolveLoginOrganisation(ctx context.Context, userID, organisationSlug string) (Organisation, error) {
	if organisationSlug != "" {
		row, err := s.store.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{UserID: id.MustParse(userID), Slug: organisationSlug})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Organisation{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
			}
			return Organisation{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageFindOrganisation))
		}
		org := organisationFromFindOrganisationRow(row)
		if org == nil {
			return Organisation{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
		}
		return *org, nil
	}
	rows, err := s.store.ListOrganisationsForUser(ctx, id.MustParse(userID))
	if err != nil {
		return Organisation{}, apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowLogin), apperrors.WithStage(apperrors.StageListOrganisations))
	}
	if len(rows) == 0 {
		return Organisation{}, apperrors.Forbidden(i18n.Msg(i18n.CodeAuthOrganisationNotFound))
	}
	return organisationFromListOrganisationRow(rows[0]), nil
}
func (s *Service) newSession(ctx context.Context, userID, organisationID, ip, userAgent string) (string, error) {
	token := s.idg.New()
	now := s.clock.Now().UTC()
	if err := s.store.InsertSession(ctx, sqlcgen.InsertSessionParams{ID: id.MustParse(token), UserID: id.MustParse(userID), ActiveOrganisationID: id.MustParse(organisationID), TokenHash: security.HashToken(token), IpAddress: ip, UserAgent: userAgent, IdleExpiresAt: sqltype.Timestamptz(now.Add(s.authConfig.SessionIdleTimeout)), ExpiresAt: sqltype.Timestamptz(now.Add(s.authConfig.SessionMaxLifetime)), CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowNewSession), apperrors.WithStage(apperrors.StageCreateSession))
	}
	return token, nil
}

func (s *Service) sessionFromContext(ctx context.Context, rawToken string) (*SessionWithUser, error) {
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
