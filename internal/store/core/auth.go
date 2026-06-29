package core

import (
	"context"
	"database/sql"
	"errors"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

func (s *Store) CreateFirstUser(ctx context.Context, in auth.CreateUserInput) (auth.User, error) {
	var user auth.User
	err := s.WithTx(ctx, func(tx *Store) error {
		count, err := tx.Query().CountUsers(ctx)
		if err != nil {
			return apperrors.Annotate(err,
				apperrors.WithOperation(apperrors.OperationCountUsers),
				apperrors.WithStage(apperrors.StageSetupFirstUser),
			)
		}
		if count > 0 {
			return apperrors.Conflict(i18n.Msg(i18n.CodeAuthSetupCompleted, i18n.P("resource", "users"), i18n.P("count", int(count))))
		}
		if err := tx.Query().InsertUser(ctx, InsertUserParams{
			ID:              in.ID,
			Email:           in.Email,
			DisplayName:     in.DisplayName,
			EmailVerifiedAt: in.Now.UTC(),
			EmailVerified:   true,
			CreatedAt:       in.Now.UTC(),
			UpdatedAt:       in.Now.UTC(),
		}); err != nil {
			return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationInsertUser), apperrors.WithStage(apperrors.StageSetupFirstUser))
		}
		if err := tx.Query().InsertAccount(ctx, InsertAccountParams{
			ID:                in.AccountID,
			UserID:            in.ID,
			Provider:          string(auth.AccountProviderPassword),
			ProviderAccountID: in.Email,
			PasswordHash:      in.PasswordHash,
			CreatedAt:         in.Now.UTC(),
			UpdatedAt:         in.Now.UTC(),
		}); err != nil {
			return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationInsertAccount), apperrors.WithStage(apperrors.StageSetupFirstUser))
		}
		if err := tx.Query().InsertOrganisation(ctx, InsertOrganisationParams{
			ID:        in.OrganisationID,
			Name:      in.OrganisationName,
			Slug:      in.OrganisationSlug,
			CreatedAt: in.Now.UTC(),
			UpdatedAt: in.Now.UTC(),
		}); err != nil {
			return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationInsertOrganisation), apperrors.WithStage(apperrors.StageSetupFirstUser))
		}
		if err := tx.Query().InsertOrganisationMember(ctx, InsertOrganisationMemberParams{
			ID:             in.OrganisationMemberID,
			OrganisationID: in.OrganisationID,
			UserID:         in.ID,
			Role:           string(auth.RoleOwner),
			CreatedAt:      in.Now.UTC(),
			UpdatedAt:      in.Now.UTC(),
		}); err != nil {
			return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationInsertOrganisationMember), apperrors.WithStage(apperrors.StageSetupFirstUser))
		}
		user = auth.User{ID: in.ID, Email: in.Email, DisplayName: in.DisplayName}
		return nil
	})
	return user, err
}

func (s *Store) FindUserByEmail(ctx context.Context, email string) (*auth.User, error) {
	row, err := s.Query().FindUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindUserByEmail))
	}
	return userFromRow(row), nil
}

func (s *Store) FindPasswordAccountByUserID(ctx context.Context, userID string) (*auth.Account, error) {
	row, err := s.Query().FindPasswordAccountByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindPasswordAccountByUserID))
	}
	return accountFromRow(row), nil
}

func (s *Store) FindUserByProviderAccount(ctx context.Context, provider, providerAccountID string) (*auth.User, error) {
	row, err := s.Query().FindUserByProviderAccount(ctx, provider, providerAccountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindUserByProviderAccount))
	}
	return userFromRow(row), nil
}

func (s *Store) ListOrganisationsForUser(ctx context.Context, userID string) ([]auth.Organisation, error) {
	rows, err := s.Query().ListOrganisationsForUser(ctx, userID)
	if err != nil {
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationListOrganisationsForUser))
	}
	out := make([]auth.Organisation, 0, len(rows))
	for _, row := range rows {
		out = append(out, organisationFromRow(row))
	}
	return out, nil
}

func (s *Store) FindOrganisationBySlugForUser(ctx context.Context, userID, slug string) (*auth.Organisation, error) {
	row, err := s.Query().FindOrganisationBySlugForUser(ctx, userID, slug)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindOrganisationBySlugForUser))
	}
	org := organisationFromRow(row)
	return &org, nil
}

func (s *Store) FindOrganisationByIDForUser(ctx context.Context, userID, organisationID string) (*auth.Organisation, error) {
	row, err := s.Query().FindOrganisationByIDForUser(ctx, userID, organisationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindOrganisationByIDForUser))
	}
	org := organisationFromRow(row)
	return &org, nil
}

func (s *Store) UpdateUserPasswordHash(ctx context.Context, in auth.UpdateUserPasswordHashInput) error {
	if err := s.Query().UpdateUserPasswordHash(ctx, UpdateUserPasswordHashParams{
		UserID:       in.UserID,
		PasswordHash: in.PasswordHash,
		UpdatedAt:    in.Now.UTC(),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationUpdateUserPasswordHash))
	}
	return nil
}

func (s *Store) CreateSession(ctx context.Context, in auth.CreateSessionInput) (auth.Session, error) {
	if err := s.Query().InsertSession(ctx, InsertSessionParams{
		ID:                   in.ID,
		UserID:               in.UserID,
		ActiveOrganisationID: in.OrganisationID,
		TokenHash:            in.TokenHash,
		IpAddress:            in.IP,
		UserAgent:            in.UserAgent,
		LastSeenAt:           in.Now.UTC(),
		IdleExpiresAt:        in.IdleExpiresAt.UTC(),
		ExpiresAt:            in.ExpiresAt.UTC(),
		CreatedAt:            in.Now.UTC(),
		UpdatedAt:            in.Now.UTC(),
	}); err != nil {
		return auth.Session{}, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationCreateSession))
	}
	return auth.Session{ID: in.ID}, nil
}

func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.SessionWithUser, error) {
	row, err := s.Query().GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationGetSessionByTokenHash))
	}
	return &auth.SessionWithUser{
		SessionID:        row.ID,
		UserID:           row.UserID,
		Email:            row.Email,
		DisplayName:      row.DisplayName,
		OrganisationID:   row.ActiveOrganisationID,
		OrganisationName: row.OrganisationName,
		OrganisationSlug: row.OrganisationSlug,
		Role:             auth.Role(row.Role),
		IdleExpiresAt:    row.IdleExpiresAt.UTC(),
		ExpiresAt:        row.ExpiresAt.UTC(),
		RevokedAt:        row.RevokedAt,
	}, nil
}

func (s *Store) RevokeSessionByTokenHash(ctx context.Context, tokenHash string) error {
	now := time.Now().UTC()
	if err := s.Query().RevokeSessionByTokenHash(ctx, RevokeSessionParams{
		RevokedAt: now,
		UpdatedAt: now,
		TokenHash: tokenHash,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationRevokeSessionByTokenHash), apperrors.WithTokenHash(apperrors.TokenHash(tokenHash)))
	}
	return nil
}

func (s *Store) RevokeSessionsForUser(ctx context.Context, userID string, now time.Time) error {
	if err := s.Query().RevokeSessionsForUser(ctx, RevokeSessionsForUserParams{
		RevokedAt: now.UTC(),
		UpdatedAt: now.UTC(),
		UserID:    userID,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationRevokeSessionsForUser))
	}
	return nil
}

func (s *Store) CreateVerificationToken(ctx context.Context, token auth.VerificationToken) error {
	if err := s.Query().InsertVerificationToken(ctx, InsertVerificationTokenParams{
		ID:          token.ID,
		UserID:      token.UserID,
		UserIDValid: token.UserIDValid,
		Email:       token.Email,
		Purpose:     token.Purpose,
		TokenHash:   token.TokenHash,
		ExpiresAt:   token.ExpiresAt.UTC(),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationInsertVerificationToken))
	}
	return nil
}

func (s *Store) FindVerificationToken(ctx context.Context, tokenHash, purpose string) (*auth.VerificationToken, error) {
	row, err := s.Query().FindVerificationToken(ctx, tokenHash, purpose)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindVerificationToken))
	}
	return &auth.VerificationToken{
		ID:              row.ID,
		UserID:          row.UserID,
		UserIDValid:     row.UserIDValid,
		Email:           row.Email,
		Purpose:         row.Purpose,
		TokenHash:       row.TokenHash,
		ExpiresAt:       row.ExpiresAt,
		ConsumedAt:      row.ConsumedAt,
		ConsumedAtValid: row.ConsumedAtValid,
	}, nil
}

func (s *Store) ConsumeVerificationToken(ctx context.Context, tokenID string, now time.Time) error {
	if err := s.Query().ConsumeVerificationToken(ctx, ConsumeVerificationTokenParams{
		ID:         tokenID,
		ConsumedAt: now.UTC(),
		UpdatedAt:  now.UTC(),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationConsumeVerificationToken))
	}
	return nil
}

func (s *Store) SaveTwoFactor(ctx context.Context, in auth.TwoFactor) error {
	now := time.Now().UTC()
	if err := s.Query().SaveTwoFactor(ctx, SaveTwoFactorParams{
		ID:        in.ID,
		UserID:    in.UserID,
		Secret:    in.Secret,
		Verified:  in.Verified,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationSaveTwoFactor))
	}
	return nil
}

func (s *Store) FindTwoFactorByUserID(ctx context.Context, userID string) (*auth.TwoFactor, error) {
	row, err := s.Query().FindTwoFactorByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationFindTwoFactorByUserID))
	}
	return &auth.TwoFactor{
		ID:                      row.ID,
		UserID:                  row.UserID,
		Secret:                  row.Secret,
		Verified:                row.Verified,
		LastUsedStep:            row.LastUsedStep,
		LastUsedStepValid:       row.LastUsedStepValid,
		FailedVerificationCount: row.FailedVerificationCount,
		LockedUntil:             row.LockedUntil,
		LockedUntilValid:        row.LockedUntilValid,
	}, nil
}

func (s *Store) ConfirmTwoFactor(ctx context.Context, userID string, step int64, now time.Time) error {
	if err := s.Query().ConfirmTwoFactor(ctx, UseTwoFactorParams{UserID: userID, LastUsedStep: step, UpdatedAt: now.UTC()}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationConfirmTwoFactor))
	}
	return nil
}

func (s *Store) MarkTwoFactorUsed(ctx context.Context, userID string, step int64, now time.Time) error {
	if err := s.Query().MarkTwoFactorUsed(ctx, UseTwoFactorParams{UserID: userID, LastUsedStep: step, UpdatedAt: now.UTC()}); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationMarkTwoFactorUsed))
	}
	return nil
}

func (s *Store) DisableTwoFactor(ctx context.Context, userID string) error {
	if err := s.Query().DisableTwoFactor(ctx, userID); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationDisableTwoFactor))
	}
	return nil
}

func userFromRow(row UserRow) *auth.User {
	return &auth.User{ID: row.ID, Email: row.Email, DisplayName: row.DisplayName}
}

func accountFromRow(row AccountRow) *auth.Account {
	return &auth.Account{
		ID:                row.ID,
		UserID:            row.UserID,
		Provider:          row.Provider,
		ProviderAccountID: row.ProviderAccountID,
		PasswordHash:      row.PasswordHash,
	}
}

func organisationFromRow(row OrganisationRow) auth.Organisation {
	return auth.Organisation{ID: row.ID, Name: row.Name, Slug: row.Slug, Role: auth.Role(row.Role)}
}
