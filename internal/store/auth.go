package store

import (
	"context"
	"database/sql"

	"github.com/google/uuid"

	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/store/types"
)

func (q *queries) CountUsers(ctx context.Context) (int64, error) {
	return q.q.CountUsers(ctx)
}

func (q *queries) InsertUser(ctx context.Context, arg types.InsertUserParams) error {
	parsedID, err := id.Parse(arg.ID)
	if err != nil {
		return err
	}
	return q.q.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:              parsedID,
		Email:           arg.Email,
		DisplayName:     arg.DisplayName,
		EmailVerifiedAt: clock.NullTime(arg.EmailVerifiedAt, arg.EmailVerified),
		CreatedAt:       arg.CreatedAt.UTC(),
		UpdatedAt:       arg.UpdatedAt.UTC(),
	})
}

func (q *queries) InsertAccount(ctx context.Context, arg types.InsertAccountParams) error {
	values, err := id.ParseFields(
		id.UUIDField{Key: "id", Value: arg.ID},
		id.UUIDField{Key: "user_id", Value: arg.UserID},
	)
	if err != nil {
		return err
	}
	return q.q.InsertAccount(ctx, sqlcgen.InsertAccountParams{
		ID:                values[0],
		UserID:            values[1],
		Provider:          arg.Provider,
		ProviderAccountID: arg.ProviderAccountID,
		PasswordHash:      nullString(arg.PasswordHash),
		CreatedAt:         arg.CreatedAt.UTC(),
		UpdatedAt:         arg.UpdatedAt.UTC(),
	})
}

func (q *queries) InsertOrganisation(ctx context.Context, arg types.InsertOrganisationParams) error {
	parsedID, err := id.Parse(arg.ID)
	if err != nil {
		return err
	}
	return q.q.InsertOrganisation(ctx, sqlcgen.InsertOrganisationParams{
		ID:        parsedID,
		Name:      arg.Name,
		Slug:      arg.Slug,
		Logo:      nullString(arg.Logo),
		CreatedAt: arg.CreatedAt.UTC(),
		UpdatedAt: arg.UpdatedAt.UTC(),
	})
}

func (q *queries) InsertOrganisationMember(ctx context.Context, arg types.InsertOrganisationMemberParams) error {
	values, err := id.ParseFields(
		id.UUIDField{Key: "id", Value: arg.ID},
		id.UUIDField{Key: "organisation_id", Value: arg.OrganisationID},
	)
	if err != nil {
		return err
	}
	userID, err := id.Parse(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{
		ID:             values[0],
		OrganisationID: values[1],
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
	parsedUserID, err := id.Parse(userID)
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
	parsedUserID, err := id.Parse(userID)
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
	parsedUserID, err := id.Parse(userID)
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
	values, err := id.ParseFields(
		id.UUIDField{Key: "user_id", Value: userID},
		id.UUIDField{Key: "organisation_id", Value: organisationID},
	)
	if err != nil {
		return types.OrganisationRow{}, err
	}
	row, err := q.q.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
		UserID: values[0],
		ID:     values[1],
	})
	if err != nil {
		return types.OrganisationRow{}, err
	}
	return organisationRow(row.ID, row.Name, row.Slug, row.Role), nil
}

func (q *queries) UpdateUserPasswordHash(ctx context.Context, arg types.UpdateUserPasswordHashParams) error {
	userID, err := id.Parse(arg.UserID)
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
	parsedID, err := id.Parse(arg.ID)
	if err != nil {
		return err
	}
	userID := uuid.NullUUID{}
	if arg.UserIDValid {
		parsedUserID, err := id.Parse(arg.UserID)
		if err != nil {
			return err
		}
		userID = uuid.NullUUID{UUID: parsedUserID, Valid: true}
	}
	return q.q.InsertVerificationToken(ctx, sqlcgen.InsertVerificationTokenParams{
		ID:        parsedID,
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
	parsedID, err := id.Parse(arg.ID)
	if err != nil {
		return err
	}
	return q.q.ConsumeVerificationToken(ctx, sqlcgen.ConsumeVerificationTokenParams{
		ConsumedAt: clock.NullTime(arg.ConsumedAt, true),
		UpdatedAt:  arg.UpdatedAt.UTC(),
		ID:         parsedID,
	})
}

func (q *queries) SaveTwoFactor(ctx context.Context, arg types.SaveTwoFactorParams) error {
	values, err := id.ParseFields(
		id.UUIDField{Key: "id", Value: arg.ID},
		id.UUIDField{Key: "user_id", Value: arg.UserID},
	)
	if err != nil {
		return err
	}
	return q.q.InsertOrReplaceTwoFactor(ctx, sqlcgen.InsertOrReplaceTwoFactorParams{
		ID:        values[0],
		UserID:    values[1],
		Secret:    arg.Secret,
		Verified:  arg.Verified,
		CreatedAt: arg.CreatedAt.UTC(),
		UpdatedAt: arg.UpdatedAt.UTC(),
	})
}

func (q *queries) FindTwoFactorByUserID(ctx context.Context, userID string) (types.TwoFactorRow, error) {
	parsedUserID, err := id.Parse(userID)
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
	userID, err := id.Parse(arg.UserID)
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
	userID, err := id.Parse(arg.UserID)
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
	parsedUserID, err := id.Parse(userID)
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
	values, err := id.ParseFields(
		id.UUIDField{Key: "id", Value: arg.ID},
		id.UUIDField{Key: "user_id", Value: arg.UserID},
	)
	if err != nil {
		return err
	}
	organisationID, err := id.Parse(arg.ActiveOrganisationID)
	if err != nil {
		return err
	}
	return q.q.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   values[0],
		UserID:               values[1],
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
	userID, err := id.Parse(arg.UserID)
	if err != nil {
		return err
	}
	return q.q.RevokeSessionsForUser(ctx, sqlcgen.RevokeSessionsForUserParams{
		RevokedAt: sql.NullTime{Time: arg.RevokedAt.UTC(), Valid: true},
		UpdatedAt: arg.UpdatedAt.UTC(),
		UserID:    userID,
	})
}

func userRow(parsedID uuid.UUID, email, displayName string) types.UserRow {
	return types.UserRow{ID: parsedID.String(), Email: email, DisplayName: displayName}
}

func accountRow(parsedID, userID uuid.UUID, provider, providerAccountID string, passwordHash sql.NullString) types.AccountRow {
	return types.AccountRow{
		ID:                parsedID.String(),
		UserID:            userID.String(),
		Provider:          provider,
		ProviderAccountID: providerAccountID,
		PasswordHash:      passwordHash.String,
	}
}

func organisationRow(parsedID uuid.UUID, name, slug, role string) types.OrganisationRow {
	return types.OrganisationRow{ID: parsedID.String(), Name: name, Slug: slug, Role: role}
}
