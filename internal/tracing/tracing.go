package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds the configuration for OpenTelemetry tracing
type Config struct {
	Enabled      bool   `json:"enabled"`
	ServiceName  string `json:"service_name"`
	Endpoint     string `json:"endpoint"`
	SamplingRate float64 `json:"sampling_rate"`
	Insecure     bool   `json:"insecure"`
	Headers      map[string]string `json:"headers"`
}

// TracerProvider wraps the OpenTelemetry tracer provider
type TracerProvider struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	config   *Config
}

// NewTracerProvider creates a new tracer provider with the given configuration
func NewTracerProvider(cfg *Config) (*TracerProvider, error) {
	if cfg == nil || !cfg.Enabled {
		// Return a no-op provider if tracing is disabled
		return &TracerProvider{
			provider: nil,
			tracer:   otel.GetTracerProvider().Tracer("tuf-server"),
			config:   cfg,
		}, nil
	}

	// Create OTLP exporter
	var exporter *otlptrace.Exporter
	var err error

	if cfg.Endpoint != "" {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(cfg.Endpoint),
		}

		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}

		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
		}

		client := otlptracehttp.NewClient(opts...)
		exporter, err = otlptrace.New(context.Background(), client)
		if err != nil {
			return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
		}
	}

	// Create resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("1.0.0"),
			attribute.String("environment", "production"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create sampler
	sampler := sdktrace.TraceIDRatioBased(cfg.SamplingRate)

	// Create tracer provider
	providerOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	}

	if exporter != nil {
		providerOpts = append(providerOpts, sdktrace.WithBatcher(exporter))
	}

	provider := sdktrace.NewTracerProvider(providerOpts...)

	// Set global tracer provider
	otel.SetTracerProvider(provider)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &TracerProvider{
		provider: provider,
		tracer:   provider.Tracer("tuf-server"),
		config:   cfg,
	}, nil
}

// Tracer returns the tracer instance
func (tp *TracerProvider) Tracer() trace.Tracer {
	return tp.tracer
}

// Shutdown gracefully shuts down the tracer provider
func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if tp.provider != nil {
		return tp.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span with the given name and options
func (tp *TracerProvider) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tp.tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the current span from the context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// AddEvent adds an event to the current span
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// SetAttributes sets attributes on the current span
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error, opts ...trace.EventOption) {
	span := SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err, opts...)
	}
}

// SetStatus sets the status of the current span
func SetStatus(ctx context.Context, code codes.Code, description string) {
	span := SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetStatus(code, description)
	}
}

// TracedFunc wraps a function with tracing
func TracedFunc(ctx context.Context, name string, fn func(context.Context) error) error {
	tracer := otel.GetTracerProvider().Tracer("tuf-server")
	ctx, span := tracer.Start(ctx, name)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	return err
}

// TraceHTTPRequest creates attributes for HTTP request tracing
func TraceHTTPRequest(method, path string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.HTTPMethod(method),
		semconv.HTTPTarget(path),
		semconv.HTTPStatusCode(statusCode),
	}
}

// TraceDBOperation creates attributes for database operation tracing
func TraceDBOperation(operation, table string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("db.operation", operation),
		attribute.String("db.table", table),
	}
}

// TraceCacheOperation creates attributes for cache operation tracing
func TraceCacheOperation(operation string, hit bool, key string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("cache.operation", operation),
		attribute.Bool("cache.hit", hit),
		attribute.String("cache.key", key),
	}
}

// TraceStorageOperation creates attributes for storage operation tracing
func TraceStorageOperation(operation, backend, path string, size int64) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("storage.operation", operation),
		attribute.String("storage.backend", backend),
		attribute.String("storage.path", path),
		attribute.Int64("storage.size", size),
	}
}

// MeasureLatency measures the latency of an operation
func MeasureLatency(start time.Time) time.Duration {
	return time.Since(start)
}

// TraceLatency adds latency as an attribute to the current span
func TraceLatency(ctx context.Context, start time.Time) {
	latency := MeasureLatency(start)
	SetAttributes(ctx, attribute.Int64("latency_ms", latency.Milliseconds()))
}