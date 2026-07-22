package auth

import (
	"context"
	"errors"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/foundation/permission"
	"github.com/sidarth-23/dinchy/internal/foundation/security"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

// resolveEmailCopy renders a catalog message at the default locale. Recipient
// locale is unknown for outbound email, so copy always uses the default language.
func resolveEmailCopy(msg i18n.Message) string {
	return i18n.Default.Resolve(i18n.Default.Match(""), msg)
}

// actionURL builds a call-to-action link from the configured base URL, a
// frontend route path, and a single-use token.
func (s *Service) actionURL(path, token string) string {
	link := url.URL{Path: path}
	if base, err := url.Parse(s.links.BaseURL); err == nil && s.links.BaseURL != "" {
		link = *base
		link.Path = path
	}
	query := link.Query()
	query.Set("token", token)
	link.RawQuery = query.Encode()
	return link.String()
}

// invitationContent builds the organization invitation email.
func (s *Service) invitationContent(organizationName, role, token string) email.Content {
	organization := i18n.P("organization", organizationName)
	return email.Content{
		Subject:  resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailInvitationSubject, organization)),
		Heading:  resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailInvitationHeading, organization)),
		Body:     resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailInvitationBody, organization, i18n.P("role", role))),
		CTALabel: resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailInvitationCta)),
		CTAURL:   s.actionURL(s.links.AcceptInvitationPath, token),
		Footer:   resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailFooter)),
	}
}

// passwordResetContent builds the password reset email.
func (s *Service) passwordResetContent(token string) email.Content {
	return email.Content{
		Subject:  resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailPasswordResetSubject)),
		Heading:  resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailPasswordResetHeading)),
		Body:     resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailPasswordResetBody)),
		CTALabel: resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailPasswordResetCta)),
		CTAURL:   s.actionURL(s.links.ResetPasswordPath, token),
		Footer:   resolveEmailCopy(i18n.Msg(i18n.CodeNotificationEmailFooter)),
	}
}

func userFromFindUserRow(row sqlcgen.FindUserByEmailRow) *User {
	if row.ID == uuid.Nil {
		return nil
	}
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName, EmailVerified: row.EmailVerifiedAt.Valid}
}

func organizationFromListOrganizationRow(row sqlcgen.ListOrganizationsForUserRow) Organization {
	return Organization{ID: row.ID.String(), Name: row.Name, Slug: row.Slug, Role: permission.Role(row.Role)}
}

func organizationFromFindOrganizationRow(row sqlcgen.FindOrganizationBySlugForUserRow) *Organization {
	if row.ID == uuid.Nil {
		return nil
	}
	org := organizationFromListOrganizationRow(sqlcgen.ListOrganizationsForUserRow(row))
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

func (s *Service) resolveLoginOrganization(ctx context.Context, userID, organizationSlug string) (Organization, error) {
	if organizationSlug != "" {
		row, err := s.store.FindOrganizationBySlugForUser(ctx, sqlcgen.FindOrganizationBySlugForUserParams{UserID: id.MustParse(userID), Slug: organizationSlug})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Organization{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthOrganizationNotFound))
			}
			return Organization{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginFindOrganization), apperrors.WithCause(err))
		}
		org := organizationFromFindOrganizationRow(row)
		if org == nil {
			return Organization{}, apperrors.BadRequest(i18n.Msg(i18n.CodeAccountAuthOrganizationNotFound))
		}
		return *org, nil
	}
	rows, err := s.store.ListOrganizationsForUser(ctx, id.MustParse(userID))
	if err != nil {
		return Organization{}, apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthLoginListOrganizations), apperrors.WithCause(err))
	}
	if len(rows) == 0 {
		return Organization{}, apperrors.Forbidden(i18n.Msg(i18n.CodeAccountAuthOrganizationNotFound))
	}
	return organizationFromListOrganizationRow(rows[0]), nil
}
