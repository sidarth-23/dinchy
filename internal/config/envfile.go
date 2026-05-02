package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

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
		if err := godotenv.Load(p); err != nil {
			return fmt.Errorf("loading env file %q: %w", p, err)
		}
		return nil
	}

	if p := xdgEnvPath(); p != "" {
		if _, err := os.Stat(p); err == nil {
			return godotenv.Load(p)
		}
	}

	const systemPath = "/etc/dinchy/dinchy.env"
	if _, err := os.Stat(systemPath); err == nil {
		return godotenv.Load(systemPath)
	}

	return nil
}

func xdgEnvPath() string {
	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		xdg = filepath.Join(home, ".config")
	}
	return filepath.Join(xdg, "dinchy", "dinchy.env")
}
