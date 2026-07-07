package config

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
}

// DefaultRedis returns the default Redis configuration used when no
// environment overrides are provided.
func DefaultRedis() RedisConfig {
	return RedisConfig{
		Addr:      "127.0.0.1:6379",
		Database:  0,
		KeyPrefix: "dinchy",
	}
}
