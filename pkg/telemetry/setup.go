package telemetry

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Protocols accepted in Config.Protocol and OTEL_EXPORTER_OTLP_PROTOCOL.
const (
	ProtocolGRPC = "grpc"
	ProtocolHTTP = "http/protobuf"
)

// Environment variables read by ConfigFromEnv in addition to the standard
// OTEL_* ones, which the OTLP exporters and resource detector read directly.
const (
	// EnvEnabled turns export on ("true", "1") or off ("false", "0").
	EnvEnabled = "DROVER_OTEL_ENABLED"
	// EnvEndpoint is a plaintext host:port collector, the variable drover's
	// pkg/telemetry already reads. OTEL_EXPORTER_OTLP_ENDPOINT wins if set.
	EnvEndpoint = "DROVER_OTEL_ENDPOINT"
	// EnvEnvironment names the deployment environment.
	EnvEnvironment = "DROVER_ENV"
)

// Config controls Setup. The zero value with a ServiceName is valid and
// exports nothing.
type Config struct {
	ServiceName    string
	ServiceVersion string
	// Environment becomes deployment.environment.name ("development",
	// "staging", "production").
	Environment string

	// Enabled turns on export. When false, Setup only installs the
	// propagators.
	Enabled bool
	// Protocol is ProtocolGRPC (default) or ProtocolHTTP.
	Protocol string
	// Endpoint is a host:port for a plaintext collector, as in
	// DROVER_OTEL_ENDPOINT. Leave empty to let the exporter read
	// OTEL_EXPORTER_OTLP_ENDPOINT, headers and TLS settings itself.
	Endpoint string
	// SampleRatio is the fraction of root spans sampled, 0 to 1. Spans with
	// a parent follow the parent's decision.
	SampleRatio float64
	// MetricInterval is how often metrics are exported. Default 60s.
	MetricInterval time.Duration

	// TraceExporter and MetricReader replace the OTLP exporters. Tests use
	// them with in-memory implementations.
	TraceExporter sdktrace.SpanExporter
	MetricReader  sdkmetric.Reader
}

// ConfigFromEnv builds a Config for service from the environment.
//
// Export is on when DROVER_OTEL_ENABLED is true, or when a collector is
// configured (OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
// or DROVER_OTEL_ENDPOINT) and DROVER_OTEL_ENABLED is not false.
// OTEL_SDK_DISABLED=true always turns it off. OTEL_SERVICE_NAME overrides
// service. OTEL_TRACES_SAMPLER_ARG sets SampleRatio (default 1).
func ConfigFromEnv(service, version string) Config {
	cfg := Config{
		ServiceName:    service,
		ServiceVersion: version,
		Environment:    firstEnv(EnvEnvironment, "ENVIRONMENT"),
		Protocol:       os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"),
		SampleRatio:    1,
	}
	if v := os.Getenv("OTEL_SERVICE_NAME"); v != "" {
		cfg.ServiceName = v
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		cfg.Endpoint = os.Getenv(EnvEndpoint)
	}
	configured := cfg.Endpoint != "" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
	switch strings.ToLower(os.Getenv(EnvEnabled)) {
	case "true", "1":
		cfg.Enabled = true
	case "false", "0":
		cfg.Enabled = false
	default:
		cfg.Enabled = configured
	}
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") {
		cfg.Enabled = false
	}
	if v := os.Getenv("OTEL_TRACES_SAMPLER_ARG"); v != "" {
		if r, err := strconv.ParseFloat(v, 64); err == nil && r >= 0 && r <= 1 {
			cfg.SampleRatio = r
		}
	}
	return cfg
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// Shutdown flushes and stops the providers Setup installed.
type Shutdown func(context.Context) error

// Setup installs the propagators and, when cfg.Enabled, global tracer and
// meter providers exporting over OTLP. Call the returned Shutdown before
// exit so buffered spans are flushed.
func Setup(ctx context.Context, cfg Config) (Shutdown, error) {
	otel.SetTextMapPropagator(Propagator())
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	if cfg.ServiceName == "" {
		return nil, errors.New("telemetry: Config.ServiceName is required")
	}
	if cfg.SampleRatio < 0 || cfg.SampleRatio > 1 {
		return nil, fmt.Errorf("telemetry: SampleRatio %v is outside 0..1", cfg.SampleRatio)
	}

	// OTEL_RESOURCE_ATTRIBUTES is read first so the explicit service
	// attributes below win over it.
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentNameKey.String(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	spanExp := cfg.TraceExporter
	if spanExp == nil {
		if spanExp, err = newSpanExporter(ctx, cfg); err != nil {
			return nil, err
		}
	}
	reader := cfg.MetricReader
	if reader == nil {
		exp, err := newMetricExporter(ctx, cfg)
		if err != nil {
			_ = spanExp.Shutdown(ctx)
			return nil, err
		}
		interval := cfg.MetricInterval
		if interval <= 0 {
			interval = time.Minute
		}
		reader = sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(interval))
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
		sdktrace.WithSpanProcessor(NewIDsProcessor()),
		sdktrace.WithBatcher(spanExp),
	)
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(reader))
	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)

	return func(ctx context.Context) error {
		return errors.Join(tp.Shutdown(ctx), mp.Shutdown(ctx))
	}, nil
}

func newSpanExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	var (
		exp sdktrace.SpanExporter
		err error
	)
	switch cfg.Protocol {
	case "", ProtocolGRPC:
		var opts []otlptracegrpc.Option
		if cfg.Endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.Endpoint), otlptracegrpc.WithInsecure())
		}
		exp, err = otlptracegrpc.New(ctx, opts...)
	case ProtocolHTTP:
		var opts []otlptracehttp.Option
		if cfg.Endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint), otlptracehttp.WithInsecure())
		}
		exp, err = otlptracehttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("telemetry: unsupported OTLP protocol %q (want %q or %q)", cfg.Protocol, ProtocolGRPC, ProtocolHTTP)
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: trace exporter: %w", err)
	}
	return exp, nil
}

func newMetricExporter(ctx context.Context, cfg Config) (sdkmetric.Exporter, error) {
	var (
		exp sdkmetric.Exporter
		err error
	)
	switch cfg.Protocol {
	case "", ProtocolGRPC:
		var opts []otlpmetricgrpc.Option
		if cfg.Endpoint != "" {
			opts = append(opts, otlpmetricgrpc.WithEndpoint(cfg.Endpoint), otlpmetricgrpc.WithInsecure())
		}
		exp, err = otlpmetricgrpc.New(ctx, opts...)
	case ProtocolHTTP:
		var opts []otlpmetrichttp.Option
		if cfg.Endpoint != "" {
			opts = append(opts, otlpmetrichttp.WithEndpoint(cfg.Endpoint), otlpmetrichttp.WithInsecure())
		}
		exp, err = otlpmetrichttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("telemetry: unsupported OTLP protocol %q (want %q or %q)", cfg.Protocol, ProtocolGRPC, ProtocolHTTP)
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: metric exporter: %w", err)
	}
	return exp, nil
}
