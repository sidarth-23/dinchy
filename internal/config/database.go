package config

// DatabaseConfig holds the persistent storage backend settings.
type DatabaseConfig struct {
	// PostgresDSN is the connection string for the PostgreSQL backend.
	PostgresDSN string `env:"DINCHY_POSTGRES_DSN" mod:"trim" validate:"required"`
}

// DefaultDatabase returns the default database configuration used when no
// environment overrides are provided.
func DefaultDatabase() DatabaseConfig {
	return DatabaseConfig{
		PostgresDSN: "postgres://postgres:postgres@localhost:5432/dinchy?sslmode=disable",
	}
}
