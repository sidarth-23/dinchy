// Package features provides shared foundations for application feature modules.
package features

import (
	"context"
	"fmt"
	"log/slog"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

// Feature identifies a feature module and provides its contextual logger.
type Feature interface {
	Name() string
	Logger(ctx context.Context) *slog.Logger
}

// Service is a Feature with common stateful-service dependencies.
type Service interface {
	Feature
	Clock() clock.Clock
	IDGenerator() *id.Generator
}

// FeatureDependencies contains dependencies shared by all feature modules.
type FeatureDependencies struct {
	Logger *slog.Logger
}

// ServiceDependencies contains dependencies shared by stateful feature services.
type ServiceDependencies struct {
	FeatureDependencies
	Clock       clock.Clock
	IDGenerator *id.Generator
}

// BaseFeature implements feature identity and context-aware logging.
type BaseFeature struct {
	name   string
	logger *slog.Logger
}

// NewBaseFeature builds a BaseFeature with a default logger when none is supplied.
func NewBaseFeature(name string, dependencies FeatureDependencies) BaseFeature {
	logger := dependencies.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return BaseFeature{name: name, logger: logger}
}

// Name returns the stable feature identifier.
func (f BaseFeature) Name() string {
	return f.name
}

// Logger returns the request logger when available, annotated with the feature name.
func (f BaseFeature) Logger(ctx context.Context) *slog.Logger {
	return logging.LoggerFromContextOr(ctx, f.logger).With(slog.String("feature", f.name))
}

// BaseService implements common dependencies for stateful feature services.
type BaseService struct {
	BaseFeature
	clock       clock.Clock
	idGenerator *id.Generator
}

// NewBaseService builds a BaseService with a required clock and default ID generator.
func NewBaseService(name string, dependencies ServiceDependencies) (BaseService, error) {
	if dependencies.Clock == nil {
		return BaseService{}, apperrors.Internal(i18n.Msg(i18n.CodeServerInternalError), apperrors.WithCause(fmt.Errorf("clock is required for feature %q", name)))
	}
	idGenerator := dependencies.IDGenerator
	if idGenerator == nil {
		idGenerator = id.NewGenerator()
	}
	return BaseService{
		BaseFeature: NewBaseFeature(name, dependencies.FeatureDependencies),
		clock:       dependencies.Clock,
		idGenerator: idGenerator,
	}, nil
}

// Clock returns the service clock.
func (s BaseService) Clock() clock.Clock {
	return s.clock
}

// IDGenerator returns the service ID generator.
func (s BaseService) IDGenerator() *id.Generator {
	return s.idGenerator
}
