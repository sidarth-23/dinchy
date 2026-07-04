package auth

import (
	"context"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

func (s *Service) StartTOTPEnrollment(ctx context.Context, userID, emailAddress string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.authConfig.TOTPIssuer, AccountName: emailAddress})
	if err != nil {
		return "", "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPEnroll))
	}
	if err := s.store.SaveTwoFactor(ctx, TwoFactor{
		ID:       s.idg.New(),
		UserID:   userID,
		Secret:   key.Secret(),
		Verified: false,
	}); err != nil {
		return "", "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPEnroll))
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) ConfirmTOTP(ctx context.Context, userID, code string) error {
	twoFactor, err := s.store.FindTwoFactorByUserID(ctx, userID)
	if err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPConfirm))
	}
	if twoFactor == nil || !totp.Validate(strings.TrimSpace(code), twoFactor.Secret) {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP))
	}
	if err := s.store.ConfirmTwoFactor(ctx, userID, totpStep(s.clock.Now()), s.clock.Now()); err != nil {
		return err
	}
	return s.recordAudit(ctx, AuditEvent{
		Category:    "security",
		Subcategory: "two_factor",
		EventType:   "auth.two_factor_enabled",
		Action:      "enable_totp",
		Outcome:     "succeeded",
		ActorUserID: userID,
		TargetType:  "user",
		TargetID:    userID,
	})
}

func (s *Service) DisableTOTP(ctx context.Context, userID string) error {
	if err := s.store.DisableTwoFactor(ctx, userID); err != nil {
		return err
	}
	return s.recordAudit(ctx, AuditEvent{
		Category:    "security",
		Subcategory: "two_factor",
		EventType:   "auth.two_factor_disabled",
		Action:      "disable_totp",
		Outcome:     "succeeded",
		ActorUserID: userID,
		TargetType:  "user",
		TargetID:    userID,
	})
}

func (s *Service) verifyTOTPForLogin(ctx context.Context, userID, code string) error {
	twoFactor, err := s.store.FindTwoFactorByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if twoFactor == nil || !twoFactor.Verified {
		return nil
	}
	step := totpStep(s.clock.Now())
	if code == "" || !totp.Validate(strings.TrimSpace(code), twoFactor.Secret) {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthTOTPRequired))
	}
	if twoFactor.LastUsedStepValid && twoFactor.LastUsedStep == step {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP))
	}
	return s.store.MarkTwoFactorUsed(ctx, userID, step, s.clock.Now())
}

func totpStep(t time.Time) int64 {
	return t.Unix() / 30
}
