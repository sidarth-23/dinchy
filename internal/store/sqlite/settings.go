package sqlite

import (
	"context"
	"time"

	"github.com/sidarth-23/dinchy/internal/domain"
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
func (s *Store) Bootstrap(ctx context.Context) (domain.BootstrapState, error) {
	count, err := s.q.CountUsers(ctx)
	if err != nil {
		return domain.BootstrapState{}, err
	}
	name, err := s.q.GetInstanceName(ctx)
	if err != nil {
		return domain.BootstrapState{}, err
	}
	return domain.BootstrapState{SetupRequired: count == 0, InstanceName: name}, nil
}
