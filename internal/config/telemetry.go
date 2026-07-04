package config

import (
	"strings"
)

const (
	DefaultTelemetryServiceName = "dinchy"
)

type TelemetryConfig struct {
	// Enabled turns on OpenTelemetry trace and log exporters.
	Enabled bool `env:"DINCHY_OTEL_ENABLED"`
	// ServiceName identifies this process in OpenTelemetry backends.
	ServiceName string `env:"DINCHY_OTEL_SERVICE_NAME"`
	// ServiceVersion is attached to telemetry resources when set.
	ServiceVersion string `env:"DINCHY_OTEL_SERVICE_VERSION"`
	// Environment names the deployment environment.
	Environment string `env:"DINCHY_OTEL_ENVIRONMENT"`
	// Endpoint is the OTLP gRPC endpoint shared by logs and traces.
	Endpoint string `env:"DINCHY_OTEL_EXPORTER_OTLP_ENDPOINT"`
	// Headers is a comma-separated key=value list sent to the collector.
	Headers string `env:"DINCHY_OTEL_EXPORTER_OTLP_HEADERS"`
	// Insecure disables TLS for the OTLP gRPC connection.
	Insecure bool `env:"DINCHY_OTEL_EXPORTER_OTLP_INSECURE"`
	// SampleRatio is the parent-based trace sampling ratio from 0 to 1.
	SampleRatio float64 `env:"DINCHY_OTEL_TRACES_SAMPLE_RATIO"`
}

func DefaultTelemetry() TelemetryConfig {
	return TelemetryConfig{
		ServiceName: DefaultTelemetryServiceName,
		SampleRatio: 1,
	}
}

func (c TelemetryConfig) HeaderMap() map[string]string {
	out := map[string]string{}
	for part := range strings.SplitSeq(c.Headers, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" {
			out[key] = value
		}
	}
	return out
}
