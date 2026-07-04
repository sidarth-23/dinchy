package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/store/sqlcgen"
)

func (s *Service) StartTOTPEnrollment(ctx context.Context, userID, emailAddress string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.authConfig.TOTPIssuer, AccountName: emailAddress})
	if err != nil {
		return "", "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPEnroll))
	}
	if err := s.store.InsertOrReplaceTwoFactor(ctx, sqlcgen.InsertOrReplaceTwoFactorParams{
		ID:        mustParseUUID(s.idg.New()),
		UserID:    mustParseUUID(userID),
		Secret:    key.Secret(),
		Verified:  false,
		CreatedAt: s.clock.Now().UTC(),
		UpdatedAt: s.clock.Now().UTC(),
	}); err != nil {
		return "", "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPEnroll))
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) ConfirmTOTP(ctx context.Context, userID, code string) error {
	twoFactorRow, err := s.store.FindTwoFactorByUserID(ctx, mustParseUUID(userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP))
		}
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPConfirm))
	}
	twoFactor := twoFactorFromFindTwoFactorRow(twoFactorRow)
	if !totp.Validate(strings.TrimSpace(code), twoFactor.Secret) {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP))
	}
	if err := s.store.ConfirmTwoFactor(ctx, sqlcgen.ConfirmTwoFactorParams{UserID: mustParseUUID(userID), LastUsedStep: sql.NullInt64{Int64: totpStep(s.clock.Now()), Valid: true}, UpdatedAt: s.clock.Now().UTC()}); err != nil {
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
	if err := s.store.DisableTwoFactor(ctx, mustParseUUID(userID)); err != nil {
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
	twoFactorRow, err := s.store.FindTwoFactorByUserID(ctx, mustParseUUID(userID))
	if err != nil {
		return err
	}
	twoFactor := twoFactorFromFindTwoFactorRow(twoFactorRow)
	if !twoFactor.Verified {
		return nil
	}
	step := totpStep(s.clock.Now())
	if code == "" || !totp.Validate(strings.TrimSpace(code), twoFactor.Secret) {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthTOTPRequired))
	}
	if twoFactor.LastUsedStepValid && twoFactor.LastUsedStep == step {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP))
	}
	return s.store.MarkTwoFactorUsed(ctx, sqlcgen.MarkTwoFactorUsedParams{UserID: mustParseUUID(userID), LastUsedStep: sql.NullInt64{Int64: step, Valid: true}, UpdatedAt: s.clock.Now().UTC()})
}

func totpStep(t time.Time) int64 {
	return t.Unix() / 30
}
