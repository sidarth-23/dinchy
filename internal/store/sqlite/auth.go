package sqlite

import (
	"context"
	"database/sql"

	"github.com/sidarth-23/dinchy/internal/store/core"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

func (q *queries) CountUsers(ctx context.Context) (int64, error) {
	return q.q.CountUsers(ctx)
}

func (q *queries) InsertUser(ctx context.Context, arg core.InsertUserParams) error {
	return q.q.InsertUser(ctx, sqlcgen.InsertUserParams{
		ID:              arg.ID,
		Email:           arg.Email,
		DisplayName:     arg.DisplayName,
		EmailVerifiedAt: nullStringTime(arg.EmailVerifiedAt, arg.EmailVerified),
		CreatedAt:       formatTime(arg.CreatedAt),
		UpdatedAt:       formatTime(arg.UpdatedAt),
	})
}

func (q *queries) InsertAccount(ctx context.Context, arg core.InsertAccountParams) error {
	return q.q.InsertAccount(ctx, sqlcgen.InsertAccountParams{
		ID:                arg.ID,
		UserID:            arg.UserID,
		Provider:          arg.Provider,
		ProviderAccountID: arg.ProviderAccountID,
		PasswordHash:      nullString(arg.PasswordHash),
		CreatedAt:         formatTime(arg.CreatedAt),
		UpdatedAt:         formatTime(arg.UpdatedAt),
	})
}

func (q *queries) InsertOrganisation(ctx context.Context, arg core.InsertOrganisationParams) error {
	return q.q.InsertOrganisation(ctx, sqlcgen.InsertOrganisationParams{
		ID:        arg.ID,
		Name:      arg.Name,
		Slug:      arg.Slug,
		Logo:      nullString(arg.Logo),
		CreatedAt: formatTime(arg.CreatedAt),
		UpdatedAt: formatTime(arg.UpdatedAt),
	})
}

func (q *queries) InsertOrganisationMember(ctx context.Context, arg core.InsertOrganisationMemberParams) error {
	return q.q.InsertOrganisationMember(ctx, sqlcgen.InsertOrganisationMemberParams{
		ID:             arg.ID,
		OrganisationID: arg.OrganisationID,
		UserID:         arg.UserID,
		Role:           arg.Role,
		CreatedAt:      formatTime(arg.CreatedAt),
		UpdatedAt:      formatTime(arg.UpdatedAt),
	})
}

func (q *queries) FindUserByEmail(ctx context.Context, email string) (core.UserRow, error) {
	row, err := q.q.FindUserByEmail(ctx, email)
	if err != nil {
		return core.UserRow{}, err
	}
	return userRow(row.ID, row.Email, row.DisplayName), nil
}

func (q *queries) FindPasswordAccountByUserID(ctx context.Context, userID string) (core.AccountRow, error) {
	row, err := q.q.FindPasswordAccountByUserID(ctx, userID)
	if err != nil {
		return core.AccountRow{}, err
	}
	return accountRow(row.ID, row.UserID, row.Provider, row.ProviderAccountID, row.PasswordHash), nil
}

func (q *queries) FindUserByProviderAccount(ctx context.Context, provider, providerAccountID string) (core.UserRow, error) {
	row, err := q.q.FindUserByProviderAccount(ctx, sqlcgen.FindUserByProviderAccountParams{
		Provider:          provider,
		ProviderAccountID: providerAccountID,
	})
	if err != nil {
		return core.UserRow{}, err
	}
	return userRow(row.ID, row.Email, row.DisplayName), nil
}

func (q *queries) ListOrganisationsForUser(ctx context.Context, userID string) ([]core.OrganisationRow, error) {
	rows, err := q.q.ListOrganisationsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]core.OrganisationRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, organisationRow(row.ID, row.Name, row.Slug, row.Role))
	}
	return out, nil
}

func (q *queries) FindOrganisationBySlugForUser(ctx context.Context, userID, slug string) (core.OrganisationRow, error) {
	row, err := q.q.FindOrganisationBySlugForUser(ctx, sqlcgen.FindOrganisationBySlugForUserParams{
		UserID: userID,
		Slug:   slug,
	})
	if err != nil {
		return core.OrganisationRow{}, err
	}
	return organisationRow(row.ID, row.Name, row.Slug, row.Role), nil
}

func (q *queries) FindOrganisationByIDForUser(ctx context.Context, userID, organisationID string) (core.OrganisationRow, error) {
	row, err := q.q.FindOrganisationByIDForUser(ctx, sqlcgen.FindOrganisationByIDForUserParams{
		UserID: userID,
		ID:     organisationID,
	})
	if err != nil {
		return core.OrganisationRow{}, err
	}
	return organisationRow(row.ID, row.Name, row.Slug, row.Role), nil
}

func (q *queries) UpdateUserPasswordHash(ctx context.Context, arg core.UpdateUserPasswordHashParams) error {
	return q.q.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{
		PasswordHash: nullString(arg.PasswordHash),
		UpdatedAt:    formatTime(arg.UpdatedAt),
		UserID:       arg.UserID,
	})
}

func (q *queries) InsertVerificationToken(ctx context.Context, arg core.InsertVerificationTokenParams) error {
	return q.q.InsertVerificationToken(ctx, sqlcgen.InsertVerificationTokenParams{
		ID:        arg.ID,
		UserID:    nullStringValid(arg.UserID, arg.UserIDValid),
		Email:     arg.Email,
		Purpose:   arg.Purpose,
		TokenHash: arg.TokenHash,
		ExpiresAt: formatTime(arg.ExpiresAt),
		CreatedAt: formatTime(arg.CreatedAt),
		UpdatedAt: formatTime(arg.UpdatedAt),
	})
}

func (q *queries) FindVerificationToken(ctx context.Context, tokenHash, purpose string) (core.VerificationTokenRow, error) {
	row, err := q.q.FindVerificationToken(ctx, sqlcgen.FindVerificationTokenParams{
		TokenHash: tokenHash,
		Purpose:   purpose,
	})
	if err != nil {
		return core.VerificationTokenRow{}, err
	}
	expiresAt, err := parseTime(row.ExpiresAt)
	if err != nil {
		return core.VerificationTokenRow{}, wrapParseErr("expires_at", err)
	}
	consumedAt, consumedAtValid, err := parseNullTime(row.ConsumedAt, "consumed_at")
	if err != nil {
		return core.VerificationTokenRow{}, err
	}
	return core.VerificationTokenRow{
		ID:              row.ID,
		UserID:          row.UserID.String,
		UserIDValid:     row.UserID.Valid,
		Email:           row.Email,
		Purpose:         row.Purpose,
		TokenHash:       row.TokenHash,
		ExpiresAt:       expiresAt,
		ConsumedAt:      consumedAt,
		ConsumedAtValid: consumedAtValid,
	}, nil
}

func (q *queries) ConsumeVerificationToken(ctx context.Context, arg core.ConsumeVerificationTokenParams) error {
	return q.q.ConsumeVerificationToken(ctx, sqlcgen.ConsumeVerificationTokenParams{
		ConsumedAt: sql.NullString{String: formatTime(arg.ConsumedAt), Valid: true},
		UpdatedAt:  formatTime(arg.UpdatedAt),
		ID:         arg.ID,
	})
}

func (q *queries) SaveTwoFactor(ctx context.Context, arg core.SaveTwoFactorParams) error {
	verified := int64(0)
	if arg.Verified {
		verified = 1
	}
	return q.q.InsertOrReplaceTwoFactor(ctx, sqlcgen.InsertOrReplaceTwoFactorParams{
		ID:        arg.ID,
		UserID:    arg.UserID,
		Secret:    arg.Secret,
		Verified:  verified,
		CreatedAt: formatTime(arg.CreatedAt),
		UpdatedAt: formatTime(arg.UpdatedAt),
	})
}

func (q *queries) FindTwoFactorByUserID(ctx context.Context, userID string) (core.TwoFactorRow, error) {
	row, err := q.q.FindTwoFactorByUserID(ctx, userID)
	if err != nil {
		return core.TwoFactorRow{}, err
	}
	lockedUntil, lockedUntilValid, err := parseNullTime(row.LockedUntil, "locked_until")
	if err != nil {
		return core.TwoFactorRow{}, err
	}
	return core.TwoFactorRow{
		ID:                      row.ID,
		UserID:                  row.UserID,
		Secret:                  row.Secret,
		Verified:                row.Verified == 1,
		LastUsedStep:            row.LastUsedStep.Int64,
		LastUsedStepValid:       row.LastUsedStep.Valid,
		FailedVerificationCount: row.FailedVerificationCount,
		LockedUntil:             lockedUntil,
		LockedUntilValid:        lockedUntilValid,
	}, nil
}

func (q *queries) ConfirmTwoFactor(ctx context.Context, arg core.UseTwoFactorParams) error {
	return q.q.ConfirmTwoFactor(ctx, sqlcgen.ConfirmTwoFactorParams{
		LastUsedStep: sql.NullInt64{Int64: arg.LastUsedStep, Valid: true},
		UpdatedAt:    formatTime(arg.UpdatedAt),
		UserID:       arg.UserID,
	})
}

func (q *queries) MarkTwoFactorUsed(ctx context.Context, arg core.UseTwoFactorParams) error {
	return q.q.MarkTwoFactorUsed(ctx, sqlcgen.MarkTwoFactorUsedParams{
		LastUsedStep: sql.NullInt64{Int64: arg.LastUsedStep, Valid: true},
		UpdatedAt:    formatTime(arg.UpdatedAt),
		UserID:       arg.UserID,
	})
}

func (q *queries) DisableTwoFactor(ctx context.Context, userID string) error {
	return q.q.DisableTwoFactor(ctx, userID)
}

func (q *queries) ListSSOProviderSettings(ctx context.Context) ([]core.SSOProviderSettingRow, error) {
	rows, err := q.q.ListSSOProviderSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]core.SSOProviderSettingRow, 0, len(rows))
	for _, row := range rows {
		createdAt, err := parseTime(row.CreatedAt)
		if err != nil {
			return nil, wrapParseErr("created_at", err)
		}
		updatedAt, err := parseTime(row.UpdatedAt)
		if err != nil {
			return nil, wrapParseErr("updated_at", err)
		}
		out = append(out, core.SSOProviderSettingRow{
			ProviderID:    row.ProviderID,
			ClientID:      row.ClientID.String,
			ClientIDValid: row.ClientID.Valid,
			Secret:        row.ClientSecret.String,
			SecretValid:   row.ClientSecret.Valid,
			CallbackURL:   row.CallbackUrl.String,
			CallbackValid: row.CallbackUrl.Valid,
			Enabled:       row.Enabled == 1,
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
		})
	}
	return out, nil
}

func (q *queries) UpsertSSOProviderSetting(ctx context.Context, arg core.UpsertSSOProviderSettingParams) error {
	enabled := int64(0)
	if arg.Enabled {
		enabled = 1
	}
	return q.q.UpsertSSOProviderSetting(ctx, sqlcgen.UpsertSSOProviderSettingParams{
		ProviderID:   arg.ProviderID,
		ClientID:     sql.NullString{String: arg.ClientID, Valid: arg.ClientIDValid},
		ClientSecret: sql.NullString{String: arg.Secret, Valid: arg.SecretValid},
		CallbackUrl:  sql.NullString{String: arg.CallbackURL, Valid: arg.CallbackValid},
		Enabled:      enabled,
		CreatedAt:    formatTime(arg.CreatedAt),
		UpdatedAt:    formatTime(arg.UpdatedAt),
	})
}

func (q *queries) InsertSession(ctx context.Context, arg core.InsertSessionParams) error {
	return q.q.InsertSession(ctx, sqlcgen.InsertSessionParams{
		ID:                   arg.ID,
		UserID:               arg.UserID,
		ActiveOrganisationID: arg.ActiveOrganisationID,
		TokenHash:            arg.TokenHash,
		IpAddress:            arg.IpAddress,
		UserAgent:            arg.UserAgent,
		LastSeenAt:           formatTime(arg.LastSeenAt),
		IdleExpiresAt:        formatTime(arg.IdleExpiresAt),
		ExpiresAt:            formatTime(arg.ExpiresAt),
		CreatedAt:            formatTime(arg.CreatedAt),
		UpdatedAt:            formatTime(arg.UpdatedAt),
	})
}

func (q *queries) GetSessionByTokenHash(ctx context.Context, tokenHash string) (core.SessionRow, error) {
	row, err := q.q.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return core.SessionRow{}, err
	}
	idle, err := parseTime(row.IdleExpiresAt)
	if err != nil {
		return core.SessionRow{}, wrapParseErr("idle_expires_at", err)
	}
	exp, err := parseTime(row.ExpiresAt)
	if err != nil {
		return core.SessionRow{}, wrapParseErr("expires_at", err)
	}
	revokedAt, revokedAtValid, err := parseNullTime(row.RevokedAt, "revoked_at")
	if err != nil {
		return core.SessionRow{}, err
	}
	return core.SessionRow{
		ID:                   row.ID,
		UserID:               row.UserID,
		Email:                row.Email,
		DisplayName:          row.DisplayName,
		ActiveOrganisationID: row.ActiveOrganisationID,
		OrganisationName:     row.OrganisationName,
		OrganisationSlug:     row.OrganisationSlug,
		Role:                 row.Role,
		IdleExpiresAt:        idle,
		ExpiresAt:            exp,
		RevokedAt:            sql.NullTime{Time: revokedAt, Valid: revokedAtValid},
	}, nil
}

func (q *queries) RevokeSessionByTokenHash(ctx context.Context, arg core.RevokeSessionParams) error {
	return q.q.RevokeSessionByTokenHash(ctx, sqlcgen.RevokeSessionByTokenHashParams{
		RevokedAt: sql.NullString{String: formatTime(arg.RevokedAt), Valid: true},
		UpdatedAt: formatTime(arg.UpdatedAt),
		TokenHash: arg.TokenHash,
	})
}

func (q *queries) RevokeSessionsForUser(ctx context.Context, arg core.RevokeSessionsForUserParams) error {
	return q.q.RevokeSessionsForUser(ctx, sqlcgen.RevokeSessionsForUserParams{
		RevokedAt: sql.NullString{String: formatTime(arg.RevokedAt), Valid: true},
		UpdatedAt: formatTime(arg.UpdatedAt),
		UserID:    arg.UserID,
	})
}

func userRow(id, email, displayName string) core.UserRow {
	return core.UserRow{ID: id, Email: email, DisplayName: displayName}
}

func accountRow(id, userID, provider, providerAccountID string, passwordHash sql.NullString) core.AccountRow {
	return core.AccountRow{
		ID:                id,
		UserID:            userID,
		Provider:          provider,
		ProviderAccountID: providerAccountID,
		PasswordHash:      passwordHash.String,
	}
}

func organisationRow(id, name, slug, role string) core.OrganisationRow {
	return core.OrganisationRow{ID: id, Name: name, Slug: slug, Role: role}
}
