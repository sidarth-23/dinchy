package auth

import (
	"database/sql"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

func nullString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func mustParseUUID(value string) uuid.UUID {
	parsed, err := id.Parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func userFromFindUserRow(row sqlcgen.FindUserByEmailRow) *User {
	if row.ID == uuid.Nil {
		return nil
	}
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName}
}

func userFromProviderAccountRow(row sqlcgen.FindUserByProviderAccountRow) *User {
	if row.ID == uuid.Nil {
		return nil
	}
	return &User{ID: row.ID.String(), Email: row.Email, DisplayName: row.DisplayName}
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

func organisationFromFindOrganisationRow(row sqlcgen.FindOrganisationBySlugForUserRow) *Organisation {
	organisation := organisationFromListOrganisationRow(sqlcgen.ListOrganisationsForUserRow{ID: row.ID, Name: row.Name, Slug: row.Slug, Role: row.Role})
	return &organisation
}

func organisationFromListOrganisationRow(row sqlcgen.ListOrganisationsForUserRow) Organisation {
	return Organisation{ID: row.ID.String(), Name: row.Name, Slug: row.Slug, Role: Role(row.Role)}
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
		IdleExpiresAt:    row.IdleExpiresAt.UTC(),
		ExpiresAt:        row.ExpiresAt.UTC(),
	}
	if row.RevokedAt.Valid {
		session.RevokedAt = row.RevokedAt
	}
	return &session
}

func twoFactorFromFindTwoFactorRow(row sqlcgen.FindTwoFactorByUserIDRow) *TwoFactor {
	twoFactor := TwoFactor{
		ID:                      row.ID.String(),
		UserID:                  row.UserID.String(),
		Secret:                  row.Secret,
		Verified:                row.Verified,
		LastUsedStep:            row.LastUsedStep.Int64,
		LastUsedStepValid:       row.LastUsedStep.Valid,
		FailedVerificationCount: row.FailedVerificationCount,
	}
	if row.LockedUntil.Valid {
		twoFactor.LockedUntil = row.LockedUntil.Time.UTC()
		twoFactor.LockedUntilValid = true
	}
	return &twoFactor
}

func verificationTokenFromFindVerificationRow(row sqlcgen.FindVerificationTokenRow) *VerificationToken {
	return &VerificationToken{
		ID:              row.ID.String(),
		UserID:          row.UserID.UUID.String(),
		UserIDValid:     row.UserID.Valid,
		Email:           row.Email,
		Purpose:         row.Purpose,
		TokenHash:       row.TokenHash,
		ExpiresAt:       row.ExpiresAt.UTC(),
		ConsumedAt:      row.ConsumedAt.Time.UTC(),
		ConsumedAtValid: row.ConsumedAt.Valid,
	}
}
