package config

type CacheConfig struct {
	// Addr is the network address for the Redis cache backend.
	Addr string `env:"DINCHY_CACHE_ADDR" mod:"trim" validate:"required,hostname_port"`
	// Username is the optional cache username.
	Username string `env:"DINCHY_CACHE_USERNAME"`
	// Password is the optional cache password.
	Password string `env:"DINCHY_CACHE_PASSWORD"`
	// Database selects the backend database or namespace when supported.
	Database int `env:"DINCHY_CACHE_DATABASE"`
	// KeyPrefix scopes all cache keys for this Dinchy instance.
	KeyPrefix string `env:"DINCHY_CACHE_KEY_PREFIX"`
}

func DefaultCache() CacheConfig {
	return CacheConfig{
		Addr:      "127.0.0.1:6379",
		Database:  0,
		KeyPrefix: "dinchy",
	}
}
