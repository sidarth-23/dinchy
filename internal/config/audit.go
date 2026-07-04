package config

type AuditConfig struct {
	// Enabled turns on durable in-app audit logging.
	Enabled bool `env:"DINCHY_AUDIT_ENABLED"`
}

func DefaultAudit() AuditConfig {
	return AuditConfig{}
}
