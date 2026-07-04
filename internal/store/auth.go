package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/store/types"
)

func (q *queries) CountUsers(ctx context.Context) (int64, error) {
	return q.q.CountUsers(ctx)
}

func (q *queries) InsertUser(ctx context.Context, arg types.InsertUserParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	return q.q.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:              id,
		Email:           arg.Email,
		DisplayName:     arg.DisplayName,
		EmailVerifiedAt: nullTime(arg.EmailVerifiedAt, arg.EmailVerified),
		CreatedAt:       arg.CreatedAt.UTC(),
		UpdatedAt:       arg.UpdatedAt.UTC(),
	})
}

func (q *queries) InsertAccount(ctx context.Context, arg types.InsertAccountParams) error {
	id, userID, err := parseTwoUUIDs(arg.ID, arg.UserID)
	if err != nil {
		return err
	}
	return q.q.InsertAccount(ctx, sqlcgen.InsertAccountParams{
		ID:                id,
		UserID:            userID,
		Provider:          arg.Provider,
		ProviderAccountID: arg.ProviderAccountID,
		PasswordHash:      nullString(arg.PasswordHash),
		CreatedAt:         arg.CreatedAt.UTC(),
		UpdatedAt:         arg.UpdatedAt.UTC(),
	})
}

func (q *queries) InsertOrganisation(ctx context.Context, arg types.InsertOrganisationParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	return q.q.InsertOrganisation(ctx, sqlcgen.InsertOrganisationParams{
		ID:        id,
		Name:      arg.Name,
		Slug:      arg.Slug,
		Logo:      nullString(arg.Logo),
		CreatedAt: arg.CreatedAt.UTC(),
		UpdatedAt: arg.UpdatedAt.UTC(),
	})
}

func (q *queries) InsertOrganisationMember(ctx context.Context, arg types.InsertOrganisationMemberParams) error {
	id, organisationID, err := parseTwoUUIDs(arg.ID, arg.OrganisationID)
	if err != nil {
		return err
	}
	userID, err := parseUUID(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{
		ID:             id,
		OrganisationID: organisationID,
		UserID:         userID,
		Role:           arg.Role,
		CreatedAt:      arg.CreatedAt.UTC(),
		UpdatedAt:      arg.UpdatedAt.UTC(),
	})
}

func (q *queries) FindUserByEmail(ctx context.Context, email string) (types.UserRow, error) {
	row, err := q.q.FindUserByEmail(ctx, email)
	if err != nil {
		return types.UserRow{}, err
	}
	return userRow(row.ID, row.Email, row.DisplayName), nil
}

func (q *queries) FindPasswordAccountByUserID(ctx context.Context, userID string) (types.AccountRow, error) {
	parsedUserID, err := parseUUID(userID)
	if err != nil {
		return types.AccountRow{}, err
	}
	row, err := q.q.FindPasswordAccountByUserID(ctx, parsedUserID)
	if err != nil {
		return types.AccountRow{}, err
	}
	return accountRow(row.ID, row.UserID, row.Provider, row.ProviderAccountID, row.PasswordHash), nil
}

func (q *queries) FindUserByProviderAccount(ctx context.Context, provider, providerAccountID string) (types.UserRow, error) {
	row, err := q.q.FindUserByProviderAccount(ctx, sqlcgen.FindUserByProviderAccountParams{
		Provider:          provider,
		ProviderAccountID: providerAccountID,
	})
	if err != nil {
		return types.UserRow{}, err
	}
	return userRow(row.ID, row.Email, row.DisplayName), nil
}

func (q *queries) ListOrganisationsForUser(ctx context.Context, userID string) ([]types.OrganisationRow, error) {
	parsedUserID, err := parseUUID(userID)
	if err != nil {
		return nil, err
	}
	rows, err := q.q.ListOrganisationsForUser(ctx, parsedUserID)
	if err != nil {
		return nil, err
	}
	out := make([]types.OrganisationRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, organisationRow(row.ID, row.Name, row.Slug, row.Role))
	}
	return out, nil
}

func (q *queries) FindOrganisationBySlugForUser(ctx context.Context, userID, slug string) (types.OrganisationRow, error) {
	parsedUserID, err := parseUUID(userID)
	if err != nil {
		return types.OrganisationRow{}, err
	}
	row, err := q.q.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{
		UserID: parsedUserID,
		Slug:   slug,
	})
	if err != nil {
		return types.OrganisationRow{}, err
	}
	return organisationRow(row.ID, row.Name, row.Slug, row.Role), nil
}

func (q *queries) FindOrganisationByIDForUser(ctx context.Context, userID, organisationID string) (types.OrganisationRow, error) {
	parsedUserID, parsedOrganisationID, err := parseTwoUUIDs(userID, organisationID)
	if err != nil {
		return types.OrganisationRow{}, err
	}
	row, err := q.q.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
		UserID: parsedUserID,
		ID:     parsedOrganisationID,
	})
	if err != nil {
		return types.OrganisationRow{}, err
	}
	return organisationRow(row.ID, row.Name, row.Slug, row.Role), nil
}

func (q *queries) UpdateUserPasswordHash(ctx context.Context, arg types.UpdateUserPasswordHashParams) error {
	userID, err := parseUUID(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
		PasswordHash: nullString(arg.PasswordHash),
		UpdatedAt:    arg.UpdatedAt.UTC(),
		UserID:       userID,
	})
}

func (q *queries) InsertVerificationToken(ctx context.Context, arg types.InsertVerificationTokenParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	userID := uuid.NullUUID{}
	if arg.UserIDValid {
		parsedUserID, err := parseUUID(arg.UserID)
		if err != nil {
			return err
		}
		userID = uuid.NullUUID{UUID: parsedUserID, Valid: true}
	}
	return q.q.InsertVerificationToken(ctx, sqlcgen.InsertVerificationTokenParams{
		ID:        id,
		UserID:    userID,
		Email:     arg.Email,
		Purpose:   arg.Purpose,
		TokenHash: arg.TokenHash,
		ExpiresAt: arg.ExpiresAt.UTC(),
		CreatedAt: arg.CreatedAt.UTC(),
		UpdatedAt: arg.UpdatedAt.UTC(),
	})
}

func (q *queries) FindVerificationToken(ctx context.Context, tokenHash, purpose string) (types.VerificationTokenRow, error) {
	row, err := q.q.FindVerificationToken(ctx, sqlcgen.FindVerificationTokenParams{TokenHash: tokenHash, Purpose: purpose})
	if err != nil {
		return types.VerificationTokenRow{}, err
	}
	return types.VerificationTokenRow{
		ID:              row.ID.String(),
		UserID:          row.UserID.UUID.String(),
		UserIDValid:     row.UserID.Valid,
		Email:           row.Email,
		Purpose:         row.Purpose,
		TokenHash:       row.TokenHash,
		ExpiresAt:       row.ExpiresAt.UTC(),
		ConsumedAt:      row.ConsumedAt.Time.UTC(),
		ConsumedAtValid: row.ConsumedAt.Valid,
	}, nil
}

func (q *queries) ConsumeVerificationToken(ctx context.Context, arg types.ConsumeVerificationTokenParams) error {
	id, err := parseUUID(arg.ID)
	if err != nil {
		return err
	}
	return q.q.ConsumeVerificationToken(ctx, sqlcgen.ConsumeVerificationTokenParams{
		ConsumedAt: sql.NullTime{Time: arg.ConsumedAt.UTC(), Valid: true},
		UpdatedAt:  arg.UpdatedAt.UTC(),
		ID:         id,
	})
}

func (q *queries) SaveTwoFactor(ctx context.Context, arg types.SaveTwoFactorParams) error {
	id, userID, err := parseTwoUUIDs(arg.ID, arg.UserID)
	if err != nil {
		return err
	}
	return q.q.InsertOrReplaceTwoFactor(ctx, sqlcgen.InsertOrReplaceTwoFactorParams{
		ID:        id,
		UserID:    userID,
		Secret:    arg.Secret,
		Verified:  arg.Verified,
		CreatedAt: arg.CreatedAt.UTC(),
		UpdatedAt: arg.UpdatedAt.UTC(),
	})
}

func (q *queries) FindTwoFactorByUserID(ctx context.Context, userID string) (types.TwoFactorRow, error) {
	parsedUserID, err := parseUUID(userID)
	if err != nil {
		return types.TwoFactorRow{}, err
	}
	row, err := q.q.FindTwoFactorByUserID(ctx, parsedUserID)
	if err != nil {
		return types.TwoFactorRow{}, err
	}
	return types.TwoFactorRow{
		ID:                      row.ID.String(),
		UserID:                  row.UserID.String(),
		Secret:                  row.Secret,
		Verified:                row.Verified,
		LastUsedStep:            row.LastUsedStep.Int64,
		LastUsedStepValid:       row.LastUsedStep.Valid,
		FailedVerificationCount: row.FailedVerificationCount,
		LockedUntil:             row.LockedUntil.Time.UTC(),
		LockedUntilValid:        row.LockedUntil.Valid,
	}, nil
}

func (q *queries) ConfirmTwoFactor(ctx context.Context, arg types.UseTwoFactorParams) error {
	userID, err := parseUUID(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.ConfirmTwoFactor(ctx, sqlcgen.ConfirmTwoFactorParams{
		LastUsedStep: sql.NullInt64{Int64: arg.LastUsedStep, Valid: true},
		UpdatedAt:    arg.UpdatedAt.UTC(),
		UserID:       userID,
	})
}

func (q *queries) MarkTwoFactorUsed(ctx context.Context, arg types.UseTwoFactorParams) error {
	userID, err := parseUUID(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.MarkTwoFactorUsed(ctx, sqlcgen.MarkTwoFactorUsedParams{
		LastUsedStep: sql.NullInt64{Int64: arg.LastUsedStep, Valid: true},
		UpdatedAt:    arg.UpdatedAt.UTC(),
		UserID:       userID,
	})
}

func (q *queries) DisableTwoFactor(ctx context.Context, userID string) error {
	parsedUserID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	return q.q.DisableTwoFactor(ctx, parsedUserID)
}

func (q *queries) ListSSOProviderSettings(ctx context.Context) ([]types.SSOProviderSettingRow, error) {
	rows, err := q.q.ListSSOProviderSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]types.SSOProviderSettingRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, types.SSOProviderSettingRow{
			ProviderID:    row.ProviderID,
			ClientID:      row.ClientID.String,
			ClientIDValid: row.ClientID.Valid,
			Secret:        row.ClientSecret.String,
			SecretValid:   row.ClientSecret.Valid,
			CallbackURL:   row.CallbackUrl.String,
			CallbackValid: row.CallbackUrl.Valid,
			Enabled:       row.Enabled,
			CreatedAt:     row.CreatedAt.UTC(),
			UpdatedAt:     row.UpdatedAt.UTC(),
		})
	}
	return out, nil
}

func (q *queries) UpsertSSOProviderSetting(ctx context.Context, arg types.UpsertSSOProviderSettingParams) error {
	return q.q.UpsertSSOProviderSetting(ctx, sqlcgen.UpsertSSOProviderSettingParams{
		ProviderID:   arg.ProviderID,
		ClientID:     sql.NullString{String: arg.ClientID, Valid: arg.ClientIDValid},
		ClientSecret: sql.NullString{String: arg.Secret, Valid: arg.SecretValid},
		CallbackUrl:  sql.NullString{String: arg.CallbackURL, Valid: arg.CallbackValid},
		Enabled:      arg.Enabled,
		CreatedAt:    arg.CreatedAt.UTC(),
		UpdatedAt:    arg.UpdatedAt.UTC(),
	})
}

func (q *queries) InsertSession(ctx context.Context, arg types.InsertSessionParams) error {
	id, userID, err := parseTwoUUIDs(arg.ID, arg.UserID)
	if err != nil {
		return err
	}
	organisationID, err := parseUUID(arg.ActiveOrganisationID)
	if err != nil {
		return err
	}
	return q.q.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   id,
		UserID:               userID,
		ActiveOrganisationID: organisationID,
		TokenHash:            arg.TokenHash,
		IpAddress:            arg.IpAddress,
		UserAgent:            arg.UserAgent,
		LastSeenAt:           arg.LastSeenAt.UTC(),
		IdleExpiresAt:        arg.IdleExpiresAt.UTC(),
		ExpiresAt:            arg.ExpiresAt.UTC(),
		CreatedAt:            arg.CreatedAt.UTC(),
		UpdatedAt:            arg.UpdatedAt.UTC(),
	})
}

func (q *queries) GetSessionByTokenHash(ctx context.Context, tokenHash string) (types.SessionRow, error) {
	row, err := q.q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return types.SessionRow{}, err
	}
	return types.SessionRow{
		ID:                   row.ID.String(),
		UserID:               row.UserID.String(),
		Email:                row.Email,
		DisplayName:          row.DisplayName,
		ActiveOrganisationID: row.ActiveOrganisationID.String(),
		OrganisationName:     row.OrganisationName,
		OrganisationSlug:     row.OrganisationSlug,
		Role:                 row.Role,
		IdleExpiresAt:        row.IdleExpiresAt.UTC(),
		ExpiresAt:            row.ExpiresAt.UTC(),
		RevokedAt:            row.RevokedAt.Time.UTC(),
		RevokedAtValid:       row.RevokedAt.Valid,
	}, nil
}

func (q *queries) RevokeSessionByTokenHash(ctx context.Context, arg types.RevokeSessionParams) error {
	return q.q.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{
		RevokedAt: sql.NullTime{Time: arg.RevokedAt.UTC(), Valid: true},
		UpdatedAt: arg.UpdatedAt.UTC(),
		TokenHash: arg.TokenHash,
	})
}

func (q *queries) RevokeSessionsForUser(ctx context.Context, arg types.RevokeSessionsForUserParams) error {
	userID, err := parseUUID(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.RevokeSessionsForUser(ctx, sqlcgen.RevokeSessionsForUserParams{
		RevokedAt: sql.NullTime{Time: arg.RevokedAt.UTC(), Valid: true},
		UpdatedAt: arg.UpdatedAt.UTC(),
		UserID:    userID,
	})
}

func parseTwoUUIDs(first, second string) (uuid.UUID, uuid.UUID, error) {
	parsedFirst, err := parseUUID(first)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, err
	}
	parsedSecond, err := parseUUID(second)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, err
	}
	return parsedFirst, parsedSecond, nil
}

func nullTime(t time.Time, valid bool) sql.NullTime {
	if !valid {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

func userRow(id uuid.UUID, email, displayName string) types.UserRow {
	return types.UserRow{ID: id.String(), Email: email, DisplayName: displayName}
}

func accountRow(id, userID uuid.UUID, provider, providerAccountID string, passwordHash sql.NullString) types.AccountRow {
	return types.AccountRow{
		ID:                id.String(),
		UserID:            userID.String(),
		Provider:          provider,
		ProviderAccountID: providerAccountID,
		PasswordHash:      passwordHash.String,
	}
}

func organisationRow(id uuid.UUID, name, slug, role string) types.OrganisationRow {
	return types.OrganisationRow{ID: id.String(), Name: name, Slug: slug, Role: role}
}
