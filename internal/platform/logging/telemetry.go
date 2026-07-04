// Package logging configures structured application logging and telemetry.
package logging

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"

	"github.com/sidarth-23/dinchy/internal/config"
)

// TelemetryRuntime owns the OpenTelemetry log and trace exporters.
type TelemetryRuntime struct {
	LogHandler slog.Handler
	closers    []io.Closer
}

// NewTelemetry creates an OpenTelemetry runtime for logs and traces.
func NewTelemetry(ctx context.Context, cfg config.TelemetryConfig) (*TelemetryRuntime, error) {
	if !cfg.Enabled {
		return &TelemetryRuntime{}, nil
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(cfg.ServiceVersion),
		semconv.DeploymentEnvironmentName(cfg.Environment),
	))
	if err != nil {
		return nil, err
	}

	traceOpts := []otlptracegrpc.Option{}
	logOpts := []otlploggrpc.Option{}
	if cfg.Endpoint != "" {
		traceOpts = append(traceOpts, otlptracegrpc.WithEndpoint(cfg.Endpoint))
		logOpts = append(logOpts, otlploggrpc.WithEndpoint(cfg.Endpoint))
	}
	if cfg.Insecure {
		traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
		logOpts = append(logOpts, otlploggrpc.WithInsecure())
	}
	headers := cfg.HeaderMap()
	if len(headers) > 0 {
		traceOpts = append(traceOpts, otlptracegrpc.WithHeaders(headers))
		logOpts = append(logOpts, otlploggrpc.WithHeaders(headers))
	}

	traceExporter, err := otlptracegrpc.New(ctx, traceOpts...)
	if err != nil {
		return nil, err
	}
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	logExporter, err := otlploggrpc.New(ctx, logOpts...)
	if err != nil {
		_ = traceProvider.Shutdown(ctx)
		return nil, err
	}
	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	global.SetLoggerProvider(logProvider)

	return &TelemetryRuntime{
		LogHandler: otelslog.NewHandler("github.com/sidarth-23/dinchy", otelslog.WithLoggerProvider(logProvider)),
		closers: []io.Closer{
			shutdownCloser{fn: traceProvider.Shutdown},
			shutdownCloser{fn: logProvider.Shutdown},
		},
	}, nil
}

// Close shuts down telemetry exporters and providers.
func (r *TelemetryRuntime) Close() error {
	if r == nil {
		return nil
	}
	for _, closer := range r.closers {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	return nil
}

type shutdownCloser struct {
	fn func(context.Context) error
}

func (c shutdownCloser) Close() error {
	return c.fn(context.Background())
}
