package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"

	"github.com/sidarth-23/dinchy/internal/config"
	apperrors "github.com/sidarth-23/dinchy/internal/foundation/errors"
	"github.com/sidarth-23/dinchy/internal/foundation/i18n"
	"github.com/sidarth-23/dinchy/internal/foundation/id"
	"github.com/sidarth-23/dinchy/internal/platform/events"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// StartTOTPEnrollment creates a new TOTP secret and stores it for the user.
func (s *Service) StartTOTPEnrollment(ctx context.Context, userID, emailAddress string) (secret, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: s.authConfig.TOTPIssuer, AccountName: emailAddress})
	if err != nil {
		return "", "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthTOTPEnroll), apperrors.WithCause(err))
	}
	if err := s.store.InsertOrReplaceTwoFactor(ctx, sqlcgen.InsertOrReplaceTwoFactorParams{
		ID:        id.MustParse(s.IDGenerator.New()),
		UserID:    id.MustParse(userID),
		Secret:    key.Secret(),
		Verified:  false,
		CreatedAt: sqltype.Timestamptz(s.Clock.Now()),
		UpdatedAt: sqltype.Timestamptz(s.Clock.Now()),
	}); err != nil {
		return "", "", apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthTOTPEnroll), apperrors.WithCause(err))
	}
	return key.Secret(), key.URL(), nil
}

// ConfirmTOTP validates the enrollment code, marks two-factor as verified, and records failures toward the lockout limit.
func (s *Service) ConfirmTOTP(ctx context.Context, userID, displayName, code string) error {
	twoFactorRow, err := s.store.FindTwoFactorByUserID(ctx, id.MustParse(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidTOTP))
		}
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthTOTPConfirm), apperrors.WithCause(err))
	}
	twoFactor := twoFactorFromFindTwoFactorRow(twoFactorRow)
	now := s.Clock.Now().UTC()
	if twoFactor.locked(now) {
		return apperrors.TooManyRequests(i18n.Msg(i18n.CodeAccountAuthTOTPLocked))
	}
	if !totp.Validate(code, twoFactor.Secret) {
		return s.recordTOTPFailure(ctx, userID, twoFactor.FailedVerificationCount, now, i18n.Msg(i18n.CodeAccountAuthInvalidTOTP))
	}
	if err := s.store.ConfirmTwoFactor(ctx, sqlcgen.ConfirmTwoFactorParams{UserID: id.MustParse(userID), LastUsedStep: sqltype.Int8(totpStep(now)), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthTOTPConfirm), apperrors.WithCause(err))
	}
	envelope, err := events.NewEnvelope(ctx, userID, "", events.NewTarget("user", userID, displayName))
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthTOTPConfirm), apperrors.WithCause(err))
	}
	return s.publishEvent(ctx, SecurityTwoFactorEnabledEvent{
		EventType: SecurityTwoFactorEnabled,
		Envelope:  envelope,
		Metadata:  NewSecurityTwoFactorEnabledMetadata(),
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

// DisableTOTP removes the user's two-factor enrollment and emits a disabled event.
func (s *Service) DisableTOTP(ctx context.Context, userID, displayName string) error {
	if err := s.store.DisableTwoFactor(ctx, id.MustParse(userID)); err != nil {
		return err
	}
	envelope, err := events.NewEnvelope(ctx, userID, "", events.NewTarget("user", userID, displayName))
	if err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthTOTPDisable), apperrors.WithCause(err))
	}
	return s.publishEvent(ctx, SecurityTwoFactorDisabledEvent{
		EventType: SecurityTwoFactorDisabled,
		Envelope:  envelope,
		Metadata:  NewSecurityTwoFactorDisabledMetadata(),
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
	now := s.Clock.Now().UTC()
	if twoFactor.locked(now) {
		return apperrors.TooManyRequests(i18n.Msg(i18n.CodeAccountAuthTOTPLocked))
	}
	step := totpStep(now)
	if code == "" || !totp.Validate(code, twoFactor.Secret) {
		msg := i18n.Msg(i18n.CodeAccountAuthInvalidTOTP)
		if code == "" {
			msg = i18n.Msg(i18n.CodeAccountAuthTOTPRequired)
		}
		return s.recordTOTPFailure(ctx, userID, twoFactor.FailedVerificationCount, now, msg)
	}
	if twoFactor.LastUsedStepValid && twoFactor.LastUsedStep == step {
		return apperrors.Unauthorized(i18n.Msg(i18n.CodeAccountAuthInvalidTOTP))
	}
	return s.store.MarkTwoFactorUsed(ctx, sqlcgen.MarkTwoFactorUsedParams{UserID: id.MustParse(userID), LastUsedStep: sqltype.Int8(step), UpdatedAt: sqltype.Timestamptz(now)})
}

func totpStep(t time.Time) int64 {
	return t.Unix() / 30
}

func (s *Service) recordTOTPFailure(ctx context.Context, userID string, currentCount int64, now time.Time, message i18n.Message) error {
	nextCount := currentCount + 1
	lockedUntil := sqltype.OptionalTimestamptz(time.Time{}, false)
	if nextCount >= config.TOTPFailureLimit {
		lockedUntil = sqltype.Timestamptz(now.Add(config.TOTPLockDuration))
	}
	if err := s.store.RegisterTwoFactorFailure(ctx, sqlcgen.RegisterTwoFactorFailureParams{
		FailureLimit: nextCount,
		LockedUntil:  lockedUntil,
		UpdatedAt:    sqltype.Timestamptz(now),
		UserID:       id.MustParse(userID),
	}); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsAuthTOTPConfirm), apperrors.WithCause(err))
	}
	if lockedUntil.Valid {
		return apperrors.TooManyRequests(i18n.Msg(i18n.CodeAccountAuthTOTPLocked))
	}
	return apperrors.Unauthorized(message)
}
