package config

import (
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/sidarth-23/dinchy/internal/platform/validation"
)

func registerValidationRules(v *validation.Validator) error {
	v.RegisterStructValidation(validateConfig, Config{})
	return nil
}

func validateConfig(sl validator.StructLevel) {
	cfg, ok := sl.Current().Interface().(Config)
	if !ok {
		return
	}

	if !cfg.Audit.Enabled {
		return
	}
	if strings.TrimSpace(string(cfg.Cache.Backend)) == "" {
		sl.ReportError(cfg.Cache.Backend, "Cache.Backend", "Cache.Backend", "oneof", string(CacheBackendRedis))
	}
	if strings.TrimSpace(cfg.EventBus.StreamName) == "" {
		sl.ReportError(cfg.EventBus.StreamName, "EventBus.StreamName", "EventBus.StreamName", "required", "")
	}
	if strings.TrimSpace(cfg.EventBus.ConsumerGroupPrefix) == "" {
		sl.ReportError(cfg.EventBus.ConsumerGroupPrefix, "EventBus.ConsumerGroupPrefix", "EventBus.ConsumerGroupPrefix", "required", "")
	}
}
