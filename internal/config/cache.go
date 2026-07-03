package config

const (
	CacheBackendDisabled  = ""
	CacheBackendRedis     = "redis"
	DefaultCacheBackend   = CacheBackendDisabled
	DefaultCacheAddr      = "127.0.0.1:6379"
	DefaultCacheDatabase  = 0
	DefaultCacheKeyPrefix = "dinchy"
)

type CacheConfig struct {
	// Backend selects the cache implementation. Empty disables the cache.
	Backend string `env:"DINCHY_CACHE_BACKEND"`
	// Addr is the network address for the configured cache backend.
	Addr string `env:"DINCHY_CACHE_ADDR"`
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
		Backend:   DefaultCacheBackend,
		Addr:      DefaultCacheAddr,
		Database:  DefaultCacheDatabase,
		KeyPrefix: DefaultCacheKeyPrefix,
	}
}
