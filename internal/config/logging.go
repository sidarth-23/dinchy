package config

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"

	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type LogLevel string

type LogFormat string

type LoggingConfig struct {
	// Level is the minimum application log level.
	Level LogLevel `env:"DINCHY_LOG_LEVEL" validate:"oneof=debug info warn error"`
	// Format selects text or JSON log output.
	Format LogFormat `env:"DINCHY_LOG_FORMAT" validate:"oneof=json text"`
	// AddSource includes source file and line metadata in logs.
	AddSource bool `env:"DINCHY_LOG_ADD_SOURCE"`
}

func DefaultLogging() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "json",
	}
}
