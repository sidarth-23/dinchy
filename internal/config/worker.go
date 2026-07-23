package config

import "time"

// WorkerConfig holds tuning for the background job scheduler.
type WorkerConfig struct {
	// Concurrency bounds how many jobs the scheduler runs at once.
	Concurrency int `env:"DINCHY_WORKER_CONCURRENCY" validate:"gt=0"`
	// ShutdownTimeout bounds how long shutdown waits for in-flight jobs to drain.
	ShutdownTimeout time.Duration `env:"DINCHY_WORKER_SHUTDOWN_TIMEOUT" validate:"gt=0"`
}

// DefaultWorker returns the default worker configuration used when no
// environment overrides are provided.
func DefaultWorker() WorkerConfig {
	return WorkerConfig{
		Concurrency:     10,
		ShutdownTimeout: 30 * time.Second,
	}
}
