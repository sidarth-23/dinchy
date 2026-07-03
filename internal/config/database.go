package config

import (
	"fmt"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

const (
	DBBackendSQLite     = "sqlite"
	DBBackendPostgres   = "postgres"
	DefaultDBBackend    = DBBackendSQLite
	DefaultSQLiteDBPath = "./dinchy.db"
)

type DatabaseConfig struct {
	// DBBackend selects the database implementation to use.
	DBBackend string `env:"DINCHY_DB_BACKEND"`
	// DBPath is the file path for the SQLite database.
	DBPath string `env:"DINCHY_DB_PATH"`
	// PostgresDSN is the connection string for the PostgreSQL backend.
	PostgresDSN string `env:"DINCHY_POSTGRES_DSN"`
}

func validateDatabaseConfig(cfg Config) error {
	switch cfg.Database.DBBackend {
	case "", DBBackendSQLite:
		if cfg.Database.DBPath == "" {
			return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_DB_PATH is required for sqlite backend")))
		}
	case DBBackendPostgres:
		if cfg.Database.PostgresDSN == "" {
			return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("DINCHY_POSTGRES_DSN is required for postgres backend")))
		}
	default:
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigValidationFailed), apperrors.WithCause(fmt.Errorf("unsupported database backend %q", cfg.Database.DBBackend)))
	}
	return nil
}
