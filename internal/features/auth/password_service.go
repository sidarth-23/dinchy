package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/security"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// VerificationPurpose identifies what a verification token authorizes.
type VerificationPurpose string

// Verification token purposes.
const (
	VerificationPurposePasswordReset VerificationPurpose = "password_reset"
)

// VerificationToken is a single-use token issued for a verification purpose such as a password reset.
type VerificationToken struct {
	ID              string
	UserID          string
	UserIDValid     bool
	Email           string
	Purpose         string
	TokenHash       string
	ExpiresAt       time.Time
	ConsumedAt      time.Time
	ConsumedAtValid bool
}

// ForgotPassword issues a password reset token and emails it, returning nil for unknown addresses to avoid account enumeration.
func (s *Service) ForgotPassword(ctx context.Context, emailAddress string) error {
	if !s.mailer.Configured() {
		return apperrors.Internal(i18n.Msg(i18n.CodeEmailNotConfigured), apperrors.WithCause(email.ErrNotConfigured))
	}
	start := s.clock.Now()
	defer waitUntil(start.Add(config.PasswordResetMinimumDuration))
	userRow, err := s.store.FindUserByEmail(ctx, emailAddress)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
		ExpiresAt: sqltype.Timestamptz(now.Add(s.authConfig.PasswordResetLifetime)),
		CreatedAt: sqltype.Timestamptz(now),
		UpdatedAt: sqltype.Timestamptz(now),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageCreateVerificationToken))
	}
	if err := s.mailer.SendPasswordReset(ctx, email.PasswordResetEmail{To: emailAddress, Token: rawToken}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageSendEmail))
	}
	return nil
}

// ResetPassword validates a reset token, sets the new password, consumes the token, and revokes the user's sessions.
func (s *Service) ResetPassword(ctx context.Context, rawToken, password string) error {
	tokenRow, err := s.store.FindVerificationToken(ctx, sqlcgen.FindVerificationTokenParams{TokenHash: security.HashToken(rawToken), Purpose: string(VerificationPurposePasswordReset)})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvalidResetToken))
		}
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageFindVerificationToken))
	}
	token := &VerificationToken{
		ID:              tokenRow.ID.String(),
		UserID:          tokenRow.UserID.UUID.String(),
		UserIDValid:     tokenRow.UserID.Valid,
		Email:           tokenRow.Email,
		Purpose:         tokenRow.Purpose,
		TokenHash:       tokenRow.TokenHash,
		ExpiresAt:       sqltype.TimeValue(tokenRow.ExpiresAt),
		ConsumedAt:      sqltype.TimeValue(tokenRow.ConsumedAt),
		ConsumedAtValid: tokenRow.ConsumedAt.Valid,
	}
	now := s.clock.Now()
	if !token.UserIDValid || token.ConsumedAtValid || now.After(token.ExpiresAt) {
		return apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvalidResetToken))
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StagePasswordHash))
	}
	if err := s.store.UpdateUserPasswordHash(ctx, sqlcgen.UpdateUserPasswordHashParams{UserID: id.MustParse(token.UserID), PasswordHash: sqltype.Text(hash), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StagePasswordHash))
	}
	if err := s.store.ConsumeVerificationToken(ctx, sqlcgen.ConsumeVerificationTokenParams{ID: id.MustParse(token.ID), ConsumedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageConsumeVerificationToken))
	}
	return s.store.RevokeSessionsForUser(ctx, sqlcgen.RevokeSessionsForUserParams{UserID: id.MustParse(token.UserID), RevokedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)})
}

func waitUntil(target time.Time) {
	if delay := time.Until(target); delay > 0 {
		time.Sleep(delay)
	}
}
