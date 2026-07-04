package config

const (
	LogFormatJSON = "json"
	LogFormatText = "text"

	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

type LoggingConfig struct {
	// Level is the minimum application log level.
	Level string `env:"DINCHY_LOG_LEVEL"`
	// Format selects text or JSON log output.
	Format string `env:"DINCHY_LOG_FORMAT"`
	// AddSource includes source file and line metadata in logs.
	AddSource bool `env:"DINCHY_LOG_ADD_SOURCE"`
}

func DefaultLogging() LoggingConfig {
	return LoggingConfig{
		Level:  LogLevelInfo,
		Format: LogFormatJSON,
	}
}
