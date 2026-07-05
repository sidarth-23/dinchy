package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/sqlutil"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

func (s *Service) ForgotPassword(ctx context.Context, emailAddress string) error {
	if !s.email.Configured() {
		return apperrors.Internal(i18n.Msg(i18n.CodeEmailNotConfigured), apperrors.WithCause(email.ErrNotConfigured))
	}
	emailAddress = transform.Email(emailAddress)
	userRow, err := s.store.FindUserByEmail(ctx, emailAddress)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageFindUser))
	}
	user := userFromFindUserRow(userRow)
	if user == nil {
		return nil
	}
	rawToken, err := security.RandomToken(32)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	tokenHash := security.HashToken(rawToken)
	now := s.clock.Now()
	if err := s.store.InsertVerificationToken(ctx, sqlcgen.InsertVerificationTokenParams{
		ID:        id.MustParse(s.idg.New()),
		UserID:    uuid.NullUUID{UUID: id.MustParse(user.ID), Valid: true},
		Email:     emailAddress,
		Purpose:   string(VerificationPurposePasswordReset),
		TokenHash: tokenHash,
		ExpiresAt: now.Add(s.authConfig.PasswordResetLifetime),
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageCreateVerificationToken))
	}
	return s.email.Send(ctx, email.Message{
		To:      emailAddress,
		Subject: "Reset your Dinchy password",
		Text:    fmt.Sprintf("Use this password reset token before it expires:\n\n%s\n", rawToken),
	})
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

func (s *Service) ResetPassword(ctx context.Context, rawToken, password string) error {
	tokenRow, err := s.store.FindVerificationToken(ctx, sqlcgen.FindVerificationTokenParams{TokenHash: security.HashToken(rawToken), Purpose: string(VerificationPurposePasswordReset)})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvalidResetToken))
		}
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageFindVerificationToken))
	}
	token := verificationTokenFromFindVerificationRow(tokenRow)
	now := s.clock.Now()
	if !token.UserIDValid || token.ConsumedAtValid || now.After(token.ExpiresAt) {
		return apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvalidResetToken))
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StagePasswordHash))
	}
	if err := s.store.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{UserID: id.MustParse(token.UserID), PasswordHash: sqlutil.NullString(hash), UpdatedAt: now}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StagePasswordHash))
	}
	if err := s.store.ConsumeVerificationToken(ctx, sqlcgen.ConsumeVerificationTokenParams{ID: id.MustParse(token.ID), ConsumedAt: sql.NullTime{Time: now.UTC(), Valid: true}, UpdatedAt: now}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageConsumeVerificationToken))
	}
	return s.store.RevokeSessionsForUser(ctx, sqlcgen.RevokeSessionsForUserParams{UserID: id.MustParse(token.UserID), RevokedAt: sql.NullTime{Time: now.UTC(), Valid: true}, UpdatedAt: now})
}
