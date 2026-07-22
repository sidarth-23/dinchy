package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sidarth-23/dinchy/internal/access/permission"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

func userFromFindUserRow(row sqlcgen.FindUserByEmailRow) *User {
	if row.ID == uuid.Nil {
		return nil
	}
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName, EmailVerified: row.EmailVerifiedAt.Valid}
}

func organisationFromListOrganisationRow(row sqlcgen.ListOrganisationsForUserRow) Organization {
	return Organization{ID: row.ID.String(), Name: row.Name, Slug: row.Slug, Role: permission.Role(row.Role)}
}

func organisationFromFindOrganisationRow(row sqlcgen.FindOrganisationBySlugForUserRow) *Organization {
	if row.ID == uuid.Nil {
		return nil
	}
	org := organisationFromListOrganisationRow(sqlcgen.ListOrganisationsForUserRow(row))
	return &org
}

func (s *Service) findUserWithPassword(ctx context.Context, emailAddress, password string) (*User, error) {
	row, err := s.store.FindUserByEmail(ctx, emailAddress)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidCredentials))
		}
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindUser), apperrors.WithCause(err))
	}
	user := userFromFindUserRow(row)
	if user == nil {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidCredentials))
	}
	accountRow, err := s.store.FindPasswordAccountByUserID(ctx, id.MustParse(user.ID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidCredentials))
		}
		return nil, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindAccount), apperrors.WithCause(err))
	}
	if !security.VerifyPassword(password, accountRow.PasswordHash.String) {
		return nil, apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidCredentials))
	}
	return user, nil
}

func (s *Service) resolveLoginOrganisation(ctx context.Context, userID, organisationSlug string) (Organization, error) {
	if organisationSlug != "" {
		row, err := s.store.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{UserID: id.MustParse(userID), Slug: organisationSlug})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Organization{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthOrganisationNotFound))
			}
			return Organization{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindOrganisation), apperrors.WithCause(err))
		}
		org := organisationFromFindOrganisationRow(row)
		if org == nil {
			return Organization{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthOrganisationNotFound))
		}
		return *org, nil
	}
	rows, err := s.store.ListOrganisationsForUser(ctx, id.MustParse(userID))
	if err != nil {
		return Organization{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginListOrganisations), apperrors.WithCause(err))
	}
	if len(rows) == 0 {
		return Organization{}, apperrors.Forbidden(i18n.Msg(i18n.CodeAccountAuthOrganisationNotFound))
	}
	return organisationFromListOrganisationRow(rows[0]), nil
}
