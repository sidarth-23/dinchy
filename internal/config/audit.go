package config

type AuditConfig struct {
	// Enabled turns on durable in-app audit logging.
	Enabled bool `env:"DINCHY_AUDIT_ENABLED"`
	// StreamName is the Redis Stream used for audit event handoff.
	StreamName string `env:"DINCHY_AUDIT_STREAM_NAME" validate:"required_if=Enabled true"`
	// ConsumerGroup is the Redis Stream consumer group that persists audit events.
	ConsumerGroup string `env:"DINCHY_AUDIT_CONSUMER_GROUP" validate:"required_if=Enabled true"`
	// ConsumerName identifies this process in the audit consumer group.
	ConsumerName string `env:"DINCHY_AUDIT_CONSUMER_NAME"`
	// BatchSize is the maximum stream messages read per consumer pass.
	BatchSize int `env:"DINCHY_AUDIT_BATCH_SIZE" validate:"gt=0"`
	// RetentionMaxLen is the approximate maximum stream length retained by Redis.
	RetentionMaxLen int64 `env:"DINCHY_AUDIT_RETENTION_MAX_LEN" validate:"gt=0"`
	// WorkerIntervalSeconds controls how often the scheduled audit worker runs.
	WorkerIntervalSeconds int64 `env:"DINCHY_AUDIT_WORKER_INTERVAL_SECONDS" validate:"gt=0"`
}

func DefaultAudit() AuditConfig {
	return AuditConfig{
		StreamName:            "audit.events",
		ConsumerGroup:         "audit-persist",
		ConsumerName:          "local",
		BatchSize:             100,
		RetentionMaxLen:       10000,
		WorkerIntervalSeconds: 5,
	}
}
