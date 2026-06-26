package core

import (
	"context"
	"time"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/features/bootstrap"
)

// EnsureDefaultSettings seeds singleton settings if they are missing.
func (s *Store) EnsureDefaultSettings(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.Query().EnsureDefaultSettings(ctx, now); err != nil {
		return apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationEnsureDefaultSettings))
	}
	return nil
}

// Bootstrap returns whether setup is required and the configured instance name.
func (s *Store) Bootstrap(ctx context.Context) (bootstrap.BootstrapState, error) {
	count, err := s.Query().CountUsers(ctx)
	if err != nil {
		return bootstrap.BootstrapState{}, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationCountUsers))
	}
	name, err := s.Query().GetInstanceName(ctx)
	if err != nil {
		return bootstrap.BootstrapState{}, apperrors.Annotate(err, apperrors.WithOperation(apperrors.OperationGetInstanceName))
	}
	return bootstrap.BootstrapState{SetupRequired: count == 0, InstanceName: name}, nil
}
