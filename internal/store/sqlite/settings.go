package sqlite

import (
	"context"
	"time"

	"github.com/sidarth-23/dinchy/internal/features/bootstrap"
	"github.com/sidarth-23/dinchy/internal/store/sqlite/sqlcgen"
)

// ensureDefaultSettings seeds the singleton app_settings row if it does not exist.
func (s *Store) ensureDefaultSettings(ctx context.Context) error {
	now := tsFormat(time.Now().UTC())
	return s.q.EnsureDefaultSettings(ctx, sqlcgen.EnsureDefaultSettingsParams{
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// Bootstrap returns whether first-user setup is required and the current instance name.
func (s *Store) Bootstrap(ctx context.Context) (bootstrap.BootstrapState, error) {
	count, err := s.q.CountUsers(ctx)
	if err != nil {
		return bootstrap.BootstrapState{}, err
	}
	name, err := s.q.GetInstanceName(ctx)
	if err != nil {
		return bootstrap.BootstrapState{}, err
	}
	return bootstrap.BootstrapState{SetupRequired: count == 0, InstanceName: name}, nil
}
