package store

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// EnsureDefaultSettings seeds singleton settings if they are missing.
func (s *Store) EnsureDefaultSettings(ctx context.Context) error {
	now := time.Now().UTC()
	if err := sqlcgen.New(s.pool).EnsureDefaultSettings(ctx, sqlcgen.EnsureDefaultSettingsParams{CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeDiagnosticsStoreEnsureDefaultSettings), apperrors.WithCause(err))
	}
	return nil
}
