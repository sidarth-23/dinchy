package auth

import (
	"database/sql"
	"time"

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

func verificationTokenFromRow(idValue uuid.UUID, userID uuid.NullUUID, email, purpose, tokenHash string, expiresAt time.Time, consumedAt sql.NullTime) VerificationToken {
	token := VerificationToken{
		ID:          idValue.String(),
		UserID:      userID.UUID.String(),
		UserIDValid: userID.Valid,
		Email:       email,
		Purpose:     purpose,
		TokenHash:   tokenHash,
		ExpiresAt:   expiresAt.UTC(),
	}
	if consumedAt.Valid {
		token.ConsumedAt = consumedAt.Time.UTC()
		token.ConsumedAtValid = true
	}
	return token
}

func twoFactorFromRow(idValue, userID uuid.UUID, secret string, verified bool, lastUsedStep sql.NullInt64, failedVerificationCount int64, lockedUntil sql.NullTime) TwoFactor {
	twoFactor := TwoFactor{
		ID:                      idValue.String(),
		UserID:                  userID.String(),
		Secret:                  secret,
		Verified:                verified,
		LastUsedStep:            lastUsedStep.Int64,
		LastUsedStepValid:       lastUsedStep.Valid,
		FailedVerificationCount: failedVerificationCount,
	}
	if lockedUntil.Valid {
		twoFactor.LockedUntil = lockedUntil.Time.UTC()
		twoFactor.LockedUntilValid = true
	}
	return twoFactor
}

func sessionFromRow(idValue, userID uuid.UUID, email, displayName, organisationID, organisationName, organisationSlug, role string, idleExpiresAt, expiresAt time.Time, revokedAt sql.NullTime) SessionWithUser {
	session := SessionWithUser{
		SessionID:        idValue.String(),
		UserID:           userID.String(),
		Email:            email,
		DisplayName:      displayName,
		OrganisationID:   organisationID,
		OrganisationName: organisationName,
		OrganisationSlug: organisationSlug,
		Role:             Role(role),
		IdleExpiresAt:    idleExpiresAt.UTC(),
		ExpiresAt:        expiresAt.UTC(),
	}
	if revokedAt.Valid {
		session.RevokedAt = revokedAt
	}
	return session
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
