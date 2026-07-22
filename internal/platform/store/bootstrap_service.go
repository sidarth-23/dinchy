package store

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/auth"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqlcgen"
	"github.com/sidarth-23/dinchy/internal/platform/store/sqltype"
)

// EnsureDefaultSettings seeds singleton settings if they are missing.
func (s *Store) EnsureDefaultSettings(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.Query().EnsureDefaultSettings(ctx, sqlcgen.EnsureDefaultSettingsParams{CreatedAt: sqltype.Timestamptz(now), UpdatedAt: sqltype.Timestamptz(now)}); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeStoreEnsureDefaultSettings), apperrors.WithCause(err))
	}
	return nil
}

// Bootstrap returns whether setup is required and the configured instance name.
func (s *Store) Bootstrap(ctx context.Context) (auth.BootstrapState, error) {
	count, err := s.Query().CountUsers(ctx)
	if err != nil {
		return auth.BootstrapState{}, apperrors.Internal(i18n.Msg(i18n.CodeStoreCountUsers), apperrors.WithCause(err))
	}
	name, err := s.Query().GetInstanceName(ctx)
	if err != nil {
		return auth.BootstrapState{}, apperrors.Internal(i18n.Msg(i18n.CodeStoreGetInstanceName), apperrors.WithCause(err))
	}
	return auth.BootstrapState{SetupRequired: count == 0, InstanceName: name}, nil
}
