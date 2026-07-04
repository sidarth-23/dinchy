package config

const (
	DBBackendSQLite   DatabaseBackend = "sqlite"
	DBBackendPostgres DatabaseBackend = "postgres"
)

type DatabaseBackend string

type DatabaseConfig struct {
	// DBBackend selects the database implementation to use.
	DBBackend DatabaseBackend `env:"DINCHY_DB_BACKEND" validate:"oneof=sqlite postgres"`
	// DBPath is the file path for the SQLite database.
	DBPath string `env:"DINCHY_DB_PATH" validate:"required_if=DBBackend sqlite"`
	// PostgresDSN is the connection string for the PostgreSQL backend.
	PostgresDSN string `env:"DINCHY_POSTGRES_DSN" validate:"required_if=DBBackend postgres"`
}
