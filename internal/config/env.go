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

// applyModsValue walks the config value recursively and normalizes every string
// field carrying a `mod:"..."` tag using the transform registry. Running once at
// load keeps all value normalization in the config phase so consumers read
// accurate values.
func applyModsValue(v reflect.Value) error {
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Type.Kind() == reflect.Struct {
			if err := applyModsValue(v.Field(i)); err != nil {
				return err
			}
			continue
		}
		spec := field.Tag.Get("mod")
		if spec == "" {
			continue
		}
		if field.Type.Kind() != reflect.String {
			return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
				apperrors.WithCause(fmt.Errorf("mod tag %q on non-string field %q", spec, field.Name)),
				apperrors.WithFieldName(apperrors.FieldName(field.Name)),
				apperrors.WithFieldKind(apperrors.FieldKindOf(field.Type.Kind())),
			)
		}
		normalized, ok := transform.Apply(spec, v.Field(i).String())
		if !ok {
			return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
				apperrors.WithCause(fmt.Errorf("unknown mod %q on field %q", spec, field.Name)),
				apperrors.WithFieldName(apperrors.FieldName(field.Name)),
				apperrors.WithFieldKind(apperrors.FieldKindOf(field.Type.Kind())),
			)
		}
		v.Field(i).SetString(normalized)
	}
	return nil
}

// loadFromEnvValue walks the config value recursively, reads the env tag on each
// field to find the env var name, and overrides the field value only when the env
// var is non-empty. Nested config structs are descended into so a single call
// populates the whole tree.
func loadFromEnvValue(v reflect.Value) error {
	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		if field.Type.Kind() == reflect.Struct {
			if err := loadFromEnvValue(v.Field(i)); err != nil {
				return err
			}
			continue
		}
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
			v.Field(i).SetString(raw)
		case reflect.Bool:
			truthy := false
			switch strings.ToLower(raw) {
			case "1", "true", "t", "yes", "on":
				truthy = true
			}
			v.Field(i).SetBool(truthy)
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
		case reflect.Float32, reflect.Float64:
			parsed, err := strconv.ParseFloat(raw, field.Type.Bits())
			if err != nil {
				return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed),
					apperrors.WithCause(fmt.Errorf("parse float env %q for %q: %w", envKey, field.Name, err)),
					apperrors.WithFieldName(apperrors.FieldName(field.Name)),
					apperrors.WithFieldKind(apperrors.FieldKindOf(field.Type.Kind())),
				)
			}
			v.Field(i).SetFloat(parsed)
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

	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return apperrors.Internal(i18n.Msg(i18n.CodeConfigLoadFailed), apperrors.WithCause(err))
		}
		xdg = filepath.Join(home, ".config")
	}
	xdgPath := filepath.Join(xdg, "dinchy", "dinchy.env")
	if _, err := os.Stat(xdgPath); err == nil {
		return loadEnvPath(xdgPath)
	}

	const systemPath = "/etc/dinchy/dinchy.env"
	if _, err := os.Stat(systemPath); err == nil {
		return loadEnvPath(systemPath)
	}

	return nil
}
