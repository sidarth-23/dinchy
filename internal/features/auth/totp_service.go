package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
)

const (
	totpFailureLimit = int64(5)
	totpLockDuration = 15 * time.Minute
)

func (s *Service) StartTOTPEnrollment(ctx context.Context, userID, emailAddress string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.authConfig.TOTPIssuer, AccountName: emailAddress})
	if err != nil {
		return "", "", apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPEnroll))
	}
	if err := s.store.InsertOrReplaceTwoFactor(ctx, sqlcgen.InsertOrReplaceTwoFactorParams{
		ID:        id.MustParse(s.idg.New()),
		UserID:    id.MustParse(userID),
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
	twoFactorRow, err := s.store.FindTwoFactorByUserID(ctx, id.MustParse(userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP))
		}
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPConfirm))
	}
	twoFactor := twoFactorFromFindTwoFactorRow(twoFactorRow)
	now := s.clock.Now().UTC()
	if twoFactor.locked(now) {
		return apperrors.TooManyRequests(i18n.Msg(i18n.CodeAuthTOTPLocked))
	}
	if !totp.Validate(strings.TrimSpace(code), twoFactor.Secret) {
		return s.recordTOTPFailure(ctx, userID, twoFactor.FailedVerificationCount, now, i18n.Msg(i18n.CodeAuthInvalidTOTP))
	}
	if err := s.store.ConfirmTwoFactor(ctx, sqlcgen.ConfirmTwoFactorParams{UserID: id.MustParse(userID), LastUsedStep: sql.NullInt64{Int64: totpStep(now), Valid: true}, UpdatedAt: now}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPConfirm))
	}
	return s.publishEvent(ctx, events.AuthSecurityTwoFactorEnabledEvent{
		EventType: events.AuthSecurityTwoFactorEnabled,
		Envelope: events.Envelope{
			ActorUserID: userID,
			TargetType:  "user",
			TargetID:    userID,
		},
		Metadata: events.NewAuthSecurityTwoFactorEnabledMetadata(),
	})
}

func twoFactorFromFindTwoFactorRow(row sqlcgen.FindTwoFactorByUserIDRow) *TwoFactor {
	twoFactor := TwoFactor{
		ID:                      row.ID.String(),
		UserID:                  row.UserID.String(),
		Secret:                  row.Secret,
		Verified:                row.Verified,
		LastUsedStep:            row.LastUsedStep.Int64,
		LastUsedStepValid:       row.LastUsedStep.Valid,
		FailedVerificationCount: row.FailedVerificationCount,
	}
	if row.LockedUntil.Valid {
		twoFactor.LockedUntil = row.LockedUntil.Time.UTC()
		twoFactor.LockedUntilValid = true
	}
	return &twoFactor
}

func (t *TwoFactor) locked(now time.Time) bool {
	return t.LockedUntilValid && now.Before(t.LockedUntil)
}

func (s *Service) DisableTOTP(ctx context.Context, userID string) error {
	if err := s.store.DisableTwoFactor(ctx, id.MustParse(userID)); err != nil {
		return err
	}
	return s.publishEvent(ctx, events.AuthSecurityTwoFactorDisabledEvent{
		EventType: events.AuthSecurityTwoFactorDisabled,
		Envelope: events.Envelope{
			ActorUserID: userID,
			TargetType:  "user",
			TargetID:    userID,
		},
		Metadata: events.NewAuthSecurityTwoFactorDisabledMetadata(),
	})
}

func (s *Service) verifyTOTPForLogin(ctx context.Context, userID, code string) error {
	twoFactorRow, err := s.store.FindTwoFactorByUserID(ctx, id.MustParse(userID))
	if err != nil {
		return err
	}
	twoFactor := twoFactorFromFindTwoFactorRow(twoFactorRow)
	if !twoFactor.Verified {
		return nil
	}
	now := s.clock.Now().UTC()
	if twoFactor.locked(now) {
		return apperrors.TooManyRequests(i18n.Msg(i18n.CodeAuthTOTPLocked))
	}
	step := totpStep(now)
	if code == "" || !totp.Validate(strings.TrimSpace(code), twoFactor.Secret) {
		msg := i18n.Msg(i18n.CodeAuthInvalidTOTP)
		if code == "" {
			msg = i18n.Msg(i18n.CodeAuthTOTPRequired)
		}
		return s.recordTOTPFailure(ctx, userID, twoFactor.FailedVerificationCount, now, msg)
	}
	if twoFactor.LastUsedStepValid && twoFactor.LastUsedStep == step {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAuthInvalidTOTP))
	}
	return s.store.MarkTwoFactorUsed(ctx, sqlcgen.MarkTwoFactorUsedParams{UserID: id.MustParse(userID), LastUsedStep: sql.NullInt64{Int64: step, Valid: true}, UpdatedAt: now})
}

func totpStep(t time.Time) int64 {
	return t.Unix() / 30
}

func (s *Service) recordTOTPFailure(ctx context.Context, userID string, currentCount int64, now time.Time, message i18n.Message) error {
	nextCount := currentCount + 1
	lockedUntil := sql.NullTime{}
	if nextCount >= totpFailureLimit {
		lockedUntil = sql.NullTime{Time: now.Add(totpLockDuration), Valid: true}
	}
	if err := s.store.RegisterTwoFactorFailure(ctx, sqlcgen.RegisterTwoFactorFailureParams{
		FailureLimit: nextCount,
		LockedUntil:  lockedUntil,
		UpdatedAt:    now,
		UserID:       id.MustParse(userID),
	}); err != nil {
		return apperrors.Annotate(err, apperrors.WithFlow(apperrors.FlowTOTP), apperrors.WithStage(apperrors.StageTOTPConfirm))
	}
	if lockedUntil.Valid {
		return apperrors.TooManyRequests(i18n.Msg(i18n.CodeAuthTOTPLocked))
	}
	return apperrors.Unauthorized(message)
}
