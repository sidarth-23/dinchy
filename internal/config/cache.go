package config

import "time"

// CacheConfig holds settings for the optional read-through cache backed by Redis.
type CacheConfig struct {
	// Enabled turns the read-through cache on. When false, every read goes to the database.
	Enabled bool `env:"DINCHY_CACHE_ENABLED"`
	// SessionTTLCap bounds how long a resolved session principal may be cached,
	// regardless of the session's remaining idle window.
	SessionTTLCap time.Duration `env:"DINCHY_CACHE_SESSION_TTL_CAP" validate:"gt=0"`
}

// DefaultCache returns the default cache configuration used when no environment overrides are provided.
func DefaultCache() CacheConfig {
	return CacheConfig{
		Enabled:       true,
		SessionTTLCap: 5 * time.Minute,
	}
}
