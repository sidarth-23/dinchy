package auth

import (
	"context"
	"fmt"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

func (s *Service) ForgotPassword(ctx context.Context, emailAddress string) error {
	if !s.email.Configured() {
		return apperrors.Internal(i18n.Msg(i18n.CodeEmailNotConfigured), apperrors.WithCause(email.ErrNotConfigured))
	}
	emailAddress = transform.Email(emailAddress)
	user, err := s.store.FindUserByEmail(ctx, emailAddress)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageFindUser))
	}
	if user == nil {
		return nil
	}
	rawToken, tokenHash, err := generateSessionToken()
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageGenerateToken))
	}
	now := s.clock.Now()
	if err := s.store.CreateVerificationToken(ctx, VerificationToken{
		ID:          s.idg.New(),
		UserID:      user.ID,
		UserIDValid: true,
		Email:       emailAddress,
		Purpose:     string(VerificationPurposePasswordReset),
		TokenHash:   tokenHash,
		ExpiresAt:   now.Add(s.authConfig.PasswordResetLifetime),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageCreateVerificationToken))
	}
	return s.email.Send(ctx, email.Message{
		To:      emailAddress,
		Subject: "Reset your Dinchy password",
		Text:    fmt.Sprintf("Use this password reset token before it expires:\n\n%s\n", rawToken),
	})
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, password string) error {
	token, err := s.store.FindVerificationToken(ctx, hashToken(rawToken), string(VerificationPurposePasswordReset))
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageFindVerificationToken))
	}
	now := s.clock.Now()
	if token == nil || token.ConsumedAtValid || now.After(token.ExpiresAt) || !token.UserIDValid {
		return apperrors.BadRequest(i18n.Msg(i18n.CodeAuthInvalidResetToken))
	}
	hash, err := hashPassword(password)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StagePasswordHash))
	}
	if err := s.store.UpdateUserPasswordHash(ctx, UpdateUserPasswordHashInput{UserID: token.UserID, PasswordHash: hash, Now: now}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StagePasswordHash))
	}
	if err := s.store.ConsumeVerificationToken(ctx, token.ID, now); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowPasswordReset), apperrors.WithStage(apperrors.StageConsumeVerificationToken))
	}
	return s.store.RevokeSessionsForUser(ctx, token.UserID, now)
}
