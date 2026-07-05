package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	apperrors "github.com/sidarth-23/dinchy/internal/errors"
	"github.com/sidarth-23/dinchy/internal/i18n"
	"github.com/sidarth-23/dinchy/internal/platform/transform"
)

// loadFromEnv iterates Config fields, reads the env tag to find the env var name,
// and overrides the field value only when the env var is non-empty.
func loadFromEnv(cfg any) error {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := range t.NumField() {
		field := t.Field(i)
		envKey := field.Tag.Get("env")
		if envKey == "" {
			continue
		}
		raw := os.Getenv(envKey)
		if raw == "" {
			continue // keep the default
		}
		switch field.Type.Kind() {
		case reflect.String:
			switch envKey {
			case "DINCHY_CACHE_BACKEND", "DINCHY_LOG_LEVEL", "DINCHY_LOG_FORMAT":
				v.Field(i).SetString(strings.ToLower(transform.Trim(raw)))
			default:
				v.Field(i).SetString(raw)
			}
		case reflect.Bool:
			v.Field(i).SetBool(parseBool(raw))
		case reflect.Int:
			var parsed int
			if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil {
				return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
					apperrors.WithCause(fmt.Errorf("parse integer env %q for %q: %w", envKey, field.Name, err)),
					apperrors.WithFieldName(apperrors.FieldName(field.Name)),
					apperrors.WithFieldKind(apperrors.FieldKindOf(field.Type.Kind())),
				)
			}
			v.Field(i).SetInt(int64(parsed))
		case reflect.Int64:
			duration, err := time.ParseDuration(raw)
			if err != nil {
				return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
					apperrors.WithCause(fmt.Errorf("parse duration env %q for %q: %w", envKey, field.Name, err)),
					apperrors.WithFieldName(apperrors.FieldName(field.Name)),
					apperrors.WithFieldKind(apperrors.FieldKindOf(field.Type.Kind())),
				)
			}
			v.Field(i).SetInt(int64(duration))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			parsed, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
					apperrors.WithCause(fmt.Errorf("parse unsigned integer env %q for %q: %w", envKey, field.Name, err)),
					apperrors.WithFieldName(apperrors.FieldName(field.Name)),
					apperrors.WithFieldKind(apperrors.FieldKindOf(field.Type.Kind())),
				)
			}
			v.Field(i).SetUint(parsed)
		default:
			return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
				apperrors.WithCause(fmt.Errorf("unsupported env field type %q for %q", field.Type.Kind().String(), field.Name)),
				apperrors.WithFieldName(apperrors.FieldName(field.Name)),
				apperrors.WithFieldKind(apperrors.FieldKindOf(field.Type.Kind())),
			)
		}
	}
	return nil
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "1", "true", "t", "yes", "on":
		return true
	}
	return false
}

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
