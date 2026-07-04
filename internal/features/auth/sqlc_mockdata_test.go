package auth

import (
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
)

func findUserRow(id, email, displayName string) sqlcgen.FindUserByEmailRow {
	return sqlcgen.FindUserByEmailRow{ID: mustParseUUID(id), Email: email, DisplayName: displayName}
}

func passwordAccountRow(id, userID, provider, providerAccountID, passwordHash string) sqlcgen.FindPasswordAccountByUserIDRow {
	return sqlcgen.FindPasswordAccountByUserIDRow{
		ID:                mustParseUUID(id),
		UserID:            mustParseUUID(userID),
		Provider:          provider,
		ProviderAccountID: providerAccountID,
		PasswordHash:      sql.NullString{String: passwordHash, Valid: passwordHash != ""},
	}
}

func organisationRow(id, name, slug, role string) sqlcgen.ListOrganisationsForUserRow {
	return sqlcgen.ListOrganisationsForUserRow{ID: mustParseUUID(id), Name: name, Slug: slug, Role: role}
}

func organisationLookupRow(id, name, slug, role string) sqlcgen.FindOrganisationBySlugForUserRow {
	return sqlcgen.FindOrganisationBySlugForUserRow{ID: mustParseUUID(id), Name: name, Slug: slug, Role: role}
}

func sessionRow(id, userID, email, displayName, organisationID, organisationName, organisationSlug, role string, idleExpiresAt, expiresAt time.Time, revokedAt sql.NullTime) sqlcgen.GetSessionByTokenHashRow {
	return sqlcgen.GetSessionByTokenHashRow{
		ID:                   mustParseUUID(id),
		UserID:               mustParseUUID(userID),
		Email:                email,
		DisplayName:          displayName,
		ActiveOrganisationID: mustParseUUID(organisationID),
		OrganisationName:     organisationName,
		OrganisationSlug:     organisationSlug,
		Role:                 role,
		IdleExpiresAt:        idleExpiresAt,
		ExpiresAt:            expiresAt,
		RevokedAt:            revokedAt,
	}
}

func twoFactorRow(id, userID, secret string, verified bool, lastUsedStep int64, lastUsedStepValid bool, failedVerificationCount int64, lockedUntil sql.NullTime) sqlcgen.FindTwoFactorByUserIDRow {
	return sqlcgen.FindTwoFactorByUserIDRow{
		ID:                      mustParseUUID(id),
		UserID:                  mustParseUUID(userID),
		Secret:                  secret,
		Verified:                verified,
		LastUsedStep:            sql.NullInt64{Int64: lastUsedStep, Valid: lastUsedStepValid},
		FailedVerificationCount: failedVerificationCount,
		LockedUntil:             lockedUntil,
	}
}

func verificationTokenRow(id, userID, email, purpose, tokenHash string, expiresAt, consumedAt time.Time, consumedAtValid bool) sqlcgen.FindVerificationTokenRow {
	nullUserID := uuid.NullUUID{}
	if userID != "" {
		nullUserID = uuid.NullUUID{UUID: mustParseUUID(userID), Valid: true}
	}
	return sqlcgen.FindVerificationTokenRow{
		ID:         mustParseUUID(id),
		UserID:     nullUserID,
		Email:      email,
		Purpose:    purpose,
		TokenHash:  tokenHash,
		ExpiresAt:  expiresAt,
		ConsumedAt: sql.NullTime{Time: consumedAt, Valid: consumedAtValid},
	}
}
