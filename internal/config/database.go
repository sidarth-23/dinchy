package config

type DatabaseConfig struct {
	// PostgresDSN is the connection string for the PostgreSQL backend.
	PostgresDSN string `env:"DINCHY_POSTGRES_DSN" mod:"trim" validate:"required"`
}
