package config

const (
	CacheBackendDisabled CacheBackend = ""
	CacheBackendRedis    CacheBackend = "redis"
)

type CacheBackend string

type CacheConfig struct {
	// Backend selects the cache implementation. Empty disables the cache.
	Backend CacheBackend `env:"DINCHY_CACHE_BACKEND" validate:"omitempty,oneof=redis"`
	// Addr is the network address for the configured cache backend.
	Addr string `env:"DINCHY_CACHE_ADDR" validate:"required_if=Backend redis,hostname_port"`
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
		Backend:   CacheBackendDisabled,
		Addr:      "127.0.0.1:6379",
		Database:  0,
		KeyPrefix: "dinchy",
	}
}
