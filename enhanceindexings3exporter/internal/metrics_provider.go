package metrics

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// MetricsProviderConfig holds configuration for the metrics provider
type MetricsProviderConfig struct {
	// Endpoint is the OTLP/HTTP endpoint to export metrics to
	Endpoint string
	// Insecure determines whether to use HTTP instead of HTTPS
	Insecure bool
	// Headers are additional headers to send with each export request
	Headers map[string]string
	// ExportInterval is how often to export metrics (default: 10s)
	ExportInterval time.Duration
	// ServiceName is the name of the service (for resource attributes)
	ServiceName string
	// ServiceVersion is the version of the service (for resource attributes)
	ServiceVersion string
}

// MetricsProvider wraps the OpenTelemetry MeterProvider and provides lifecycle management
type MetricsProvider struct {
	provider *metric.MeterProvider
}

// deltaTemporalitySelector configures the reader to use delta temporality for all instruments.
// Delta temporality reports the change in value since the last export, which is preferred for
// many observability backends including Honeycomb.
func deltaTemporalitySelector(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.DeltaTemporality
}

// NewMetricsProvider creates a new MetricsProvider with OTLP HTTP exporter
func NewMetricsProvider(ctx context.Context, config MetricsProviderConfig) (*MetricsProvider, error) {
	// Set defaults
	if config.ExportInterval == 0 {
		config.ExportInterval = 10 * time.Second
	}
	if config.ServiceName == "" {
		config.ServiceName = "enhance-indexing-s3-exporter"
	}
	if config.ServiceVersion == "" {
		config.ServiceVersion = "0.1.0"
	}

	// Create resource with service information
	res, err := newResource(config.ServiceName, config.ServiceVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Configure OTLP HTTP exporter options
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(config.Endpoint),
	}

	if config.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	if len(config.Headers) > 0 {
		opts = append(opts, otlpmetrichttp.WithHeaders(config.Headers))
	}

	// Create OTLP HTTP exporter with delta temporality preference
	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP HTTP exporter: %w", err)
	}

	// Create periodic reader with configured interval and delta temporality
	reader := metric.NewPeriodicReader(
		exporter,
		metric.WithInterval(config.ExportInterval),
		// Configure temporality selector to prefer delta for all instruments
		metric.WithTemporalitySelector(deltaTemporalitySelector),
	)

	// Create meter provider
	provider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(reader),
	)

	return &MetricsProvider{
		provider: provider,
	}, nil
}

// newResource creates a resource with service information
func newResource(serviceName, serviceVersion string) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
}

// MeterProvider returns the underlying MeterProvider
func (mp *MetricsProvider) MeterProvider() *metric.MeterProvider {
	return mp.provider
}

// Shutdown gracefully shuts down the metrics provider, flushing any pending metrics
func (mp *MetricsProvider) Shutdown(ctx context.Context) error {
	if mp.provider == nil {
		return nil
	}
	return mp.provider.Shutdown(ctx)
}

// ForceFlush forces the metrics provider to flush any pending metrics
func (mp *MetricsProvider) ForceFlush(ctx context.Context) error {
	if mp.provider == nil {
		return nil
	}
	return mp.provider.ForceFlush(ctx)
}
