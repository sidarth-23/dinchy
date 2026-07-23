package config

import "time"

// RedisConfig holds the shared Redis backend settings for ephemeral state and event streams.
type RedisConfig struct {
	// Addr is the network address for the shared Redis backend used for ephemeral state and event streams.
	Addr string `env:"DINCHY_REDIS_ADDR" mod:"trim" validate:"required,hostname_port"`
	// Username is the optional Redis username.
	Username string `env:"DINCHY_REDIS_USERNAME"`
	// Password is the optional Redis password.
	Password string `env:"DINCHY_REDIS_PASSWORD"`
	// Database selects the backend database or namespace when supported.
	Database int `env:"DINCHY_REDIS_DATABASE"`
	// KeyPrefix scopes all Redis keys for this Dinchy instance.
	KeyPrefix string `env:"DINCHY_REDIS_KEY_PREFIX"`
	// TLS enables a TLS connection to the Redis backend.
	TLS bool `env:"DINCHY_REDIS_TLS"`
	// PoolSize is the maximum number of socket connections in the pool.
	PoolSize int `env:"DINCHY_REDIS_POOL_SIZE" validate:"gt=0"`
	// MinIdleConns is the number of idle connections kept open to avoid slow reconnects.
	MinIdleConns int `env:"DINCHY_REDIS_MIN_IDLE_CONNS" validate:"gte=0"`
	// DialTimeout bounds establishing a new connection.
	DialTimeout time.Duration `env:"DINCHY_REDIS_DIAL_TIMEOUT" validate:"gt=0"`
	// ReadTimeout bounds a single socket read.
	ReadTimeout time.Duration `env:"DINCHY_REDIS_READ_TIMEOUT" validate:"gt=0"`
	// WriteTimeout bounds a single socket write.
	WriteTimeout time.Duration `env:"DINCHY_REDIS_WRITE_TIMEOUT" validate:"gt=0"`
}

// DefaultRedis returns the default Redis configuration used when no
// environment overrides are provided.
func DefaultRedis() RedisConfig {
	return RedisConfig{
		Addr:         "127.0.0.1:6379",
		Database:     0,
		KeyPrefix:    "dinchy",
		PoolSize:     10,
		MinIdleConns: 0,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
}
