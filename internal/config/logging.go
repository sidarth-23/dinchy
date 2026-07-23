package config

// Supported log output formats and minimum log levels.
const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"

	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogLevel is the minimum severity emitted by the application logger.
type LogLevel string

// LogFormat selects text or JSON log output.
type LogFormat string

// LoggingConfig controls application log formatting and level.
type LoggingConfig struct {
	// Level is the minimum application log level.
	Level LogLevel `env:"DINCHY_LOG_LEVEL" mod:"trim,lower" validate:"oneof=trace debug info warn error"`
	// Format selects text or JSON log output.
	Format LogFormat `env:"DINCHY_LOG_FORMAT" mod:"trim,lower" validate:"oneof=json text"`
	// AddSource includes source file and line metadata in logs.
	AddSource bool `env:"DINCHY_LOG_ADD_SOURCE"`
}

// DefaultLogging returns the default logging configuration used when no
// environment overrides are provided.
func DefaultLogging() LoggingConfig {
	return LoggingConfig{
		Level:  "info",
		Format: "json",
	}
}
