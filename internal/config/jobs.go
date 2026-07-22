package config

// JobsConfig holds tuning for the durable background job queue (River).
type JobsConfig struct {
	// MaxWorkers is the maximum number of durable jobs run concurrently on the default queue.
	MaxWorkers int `env:"DINCHY_JOBS_MAX_WORKERS" validate:"gt=0"`
}

// DefaultJobs returns the default job queue configuration used when no
// environment overrides are provided.
func DefaultJobs() JobsConfig {
	return JobsConfig{
		MaxWorkers: 100,
	}
}
