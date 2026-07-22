// Package module provides shared service foundations for application modules.
package module

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/events"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/cache"
	"github.com/sidarth-23/dinchy/internal/platform/clock"
	"github.com/sidarth-23/dinchy/internal/platform/email"
	"github.com/sidarth-23/dinchy/internal/platform/id"
	"github.com/sidarth-23/dinchy/internal/platform/logging"
)

// Module identifies an application module and provides its contextual logger.
type Module interface {
	Name() string
	Logger(ctx context.Context) *slog.Logger
}

// Service provides shared infrastructure for one named application module.
type Service struct {
	ModuleName     string
	BaseLogger     *slog.Logger
	Clock          clock.Clock
	IDGenerator    *id.Generator
	Database       *pgxpool.Pool
	RedisClient    *goredis.Client
	Cache          cache.Cache
	CacheKeyer     cache.Keyer
	Mailer         *email.Mailer
	EventPublisher events.Publisher
}

// Named returns a named copy of the Service that shares its configured clients.
func (s Service) Named(moduleName string) *Service {
	s.ModuleName = moduleName
	return &s
}

// Initialize validates a Service and supplies its optional default dependencies.
func (s *Service) Initialize() error {
	if s == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("module service is required")))
	}
	if s.ModuleName == "" {
		return apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("module name is required")))
	}
	if s.Clock == nil {
		return apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("clock is required for module %q", s.ModuleName)))
	}
	if s.BaseLogger == nil {
		s.BaseLogger = slog.Default()
	}
	if s.IDGenerator == nil {
		s.IDGenerator = id.NewGenerator()
	}
	if s.Mailer == nil {
		var err error
		s.Mailer, err = email.NewMailer(email.NoopSender{}, "")
		if err != nil {
			return apperrors.Internal(i18n.Msg(i18n.CodePlatformServerInternalError), apperrors.WithCause(fmt.Errorf("create default mailer for module %q: %w", s.ModuleName, err)))
		}
	}
	return nil
}

// Name returns the stable module identifier.
func (s *Service) Name() string {
	return s.ModuleName
}

// Logger returns the request logger when available, annotated with the module name.
func (s *Service) Logger(ctx context.Context) *slog.Logger {
	return logging.LoggerFromContextOr(ctx, s.BaseLogger).With(slog.String("module", s.ModuleName))
}

var _ Module = (*Service)(nil)
