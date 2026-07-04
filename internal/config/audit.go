package config

import (
	"fmt"
	"strings"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

const (
	DefaultAuditStream        = "audit.events"
	DefaultAuditConsumerGroup = "audit-persist"
	DefaultAuditConsumerName  = "local"
)

type AuditConfig struct {
	// Enabled turns on durable in-app audit logging.
	Enabled bool `env:"DINCHY_AUDIT_ENABLED"`
	// StreamName is the Redis Stream used for audit event handoff.
	StreamName string `env:"DINCHY_AUDIT_STREAM_NAME"`
	// ConsumerGroup is the Redis Stream consumer group that persists audit events.
	ConsumerGroup string `env:"DINCHY_AUDIT_CONSUMER_GROUP"`
	// ConsumerName identifies this process in the audit consumer group.
	ConsumerName string `env:"DINCHY_AUDIT_CONSUMER_NAME"`
	// BatchSize is the maximum stream messages read per consumer pass.
	BatchSize int `env:"DINCHY_AUDIT_BATCH_SIZE"`
	// RetentionMaxLen is the approximate maximum stream length retained by Redis.
	RetentionMaxLen int64 `env:"DINCHY_AUDIT_RETENTION_MAX_LEN"`
	// WorkerIntervalSeconds controls how often the scheduled audit worker runs.
	WorkerIntervalSeconds int64 `env:"DINCHY_AUDIT_WORKER_INTERVAL_SECONDS"`
}

func DefaultAudit() AuditConfig {
	return AuditConfig{
		StreamName:            DefaultAuditStream,
		ConsumerGroup:         DefaultAuditConsumerGroup,
		ConsumerName:          DefaultAuditConsumerName,
		BatchSize:             100,
		RetentionMaxLen:       10000,
		WorkerIntervalSeconds: 5,
	}
}

func validateAuditConfig(cfg Config) error {
	if !cfg.Audit.Enabled {
		return nil
	}
	if strings.TrimSpace(cfg.Cache.Backend) != CacheBackendRedis {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_CACHE_BACKEND must be %q when DINCHY_AUDIT_ENABLED is true", CacheBackendRedis)))
	}
	if strings.TrimSpace(cfg.Audit.StreamName) == "" {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_AUDIT_STREAM_NAME is required when DINCHY_AUDIT_ENABLED is true")))
	}
	if strings.TrimSpace(cfg.Audit.ConsumerGroup) == "" {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_AUDIT_CONSUMER_GROUP is required when DINCHY_AUDIT_ENABLED is true")))
	}
	if cfg.Audit.BatchSize < 1 {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_AUDIT_BATCH_SIZE must be greater than zero")))
	}
	if cfg.Audit.WorkerIntervalSeconds < 1 {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_AUDIT_WORKER_INTERVAL_SECONDS must be greater than zero")))
	}
	return nil
}
