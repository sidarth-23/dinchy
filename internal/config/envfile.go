package config

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
)

func loadEnvPath(p string) error {
	if err := godotenv.Load(p); err != nil {
		return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed), apperrors.WithCause(err), apperrors.WithPath(apperrors.Path(p)))
	}
	return nil
}

// loadEnvFile populates the process environment from a .env file.
// Resolution order:
//  1. DINCHY_ENV_FILE (explicit path — fatal if set but missing)
//  2. $XDG_CONFIG_HOME/dinchy/dinchy.env  (defaults to ~/.config/dinchy/dinchy.env)
//  3. /etc/dinchy/dinchy.env
//  4. Nothing found — silently proceed with current environment
//
// godotenv.Load does NOT override variables already in the environment,
// so explicit Environment= entries in systemd units take precedence over the file.
func loadEnvFile() error {
	if p := os.Getenv("DINCHY_ENV_FILE"); p != "" {
		return loadEnvPath(p)
	}

	if p, err := xdgEnvPath(); err != nil {
		return apperrors.Annotate(err)
	} else if p != "" {
		if _, err := os.Stat(p); err == nil {
			return loadEnvPath(p)
		}
	}

	const systemPath = "/etc/dinchy/dinchy.env"
	if _, err := os.Stat(systemPath); err == nil {
		return loadEnvPath(systemPath)
	}

	return nil
}

func xdgEnvPath() (string, error) {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed), apperrors.WithCause(err), apperrors.WithStage(apperrors.StageResolveXDGConfigHome))
		}
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "dinchy", "dinchy.env"), nil
}
