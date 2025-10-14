package enhanceindexings3exporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// instrumentationScopeName is the distinct scope name for indexer metrics
	instrumentationScopeName = "github.com/honeycombio/enhance-indexing-s3-exporter"
)

// ExporterMetrics holds OpenTelemetry metrics for the exporter with distinct instrumentation scope
type ExporterMetrics struct {
	// OpenTelemetry metrics instruments with distinct scope
	meter          metric.Meter
	spanCountTotal metric.Int64Counter
	spanBytesTotal metric.Int64Counter
	logCountTotal  metric.Int64Counter
	logBytesTotal  metric.Int64Counter
}

// NewExporterMetrics creates a new ExporterMetrics with OpenTelemetry instrumentation
func NewExporterMetrics() (*ExporterMetrics, error) {
	// Create meter with distinct instrumentation scope
	meter := otel.Meter(instrumentationScopeName)

	// Create metric instruments
	spanCountTotal, err := meter.Int64Counter(
		"indexer_spans_total",
		metric.WithDescription("Total number of spans processed by the indexer"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create span count counter: %w", err)
	}

	spanBytesTotal, err := meter.Int64Counter(
		"indexer_span_bytes_total",
		metric.WithDescription("Total bytes of span data processed by the indexer"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create span bytes counter: %w", err)
	}

	logCountTotal, err := meter.Int64Counter(
		"indexer_logs_total",
		metric.WithDescription("Total number of log records processed by the indexer"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create log count counter: %w", err)
	}

	logBytesTotal, err := meter.Int64Counter(
		"indexer_log_bytes_total",
		metric.WithDescription("Total bytes of log data processed by the indexer"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create log bytes counter: %w", err)
	}

	return &ExporterMetrics{
		meter:          meter,
		spanCountTotal: spanCountTotal,
		spanBytesTotal: spanBytesTotal,
		logCountTotal:  logCountTotal,
		logBytesTotal:  logBytesTotal,
	}, nil
}

// Note: OpenTelemetry counters are write-only and cannot be read directly.
// Metrics are exported through the configured OpenTelemetry metrics pipeline.

// AddSpanMetrics records span count and bytes to OpenTelemetry metrics with distinct instrumentation scope
func (m *ExporterMetrics) AddSpanMetrics(ctx context.Context, count int64, bytes int64, attrs []attribute.KeyValue) {
	// Record OpenTelemetry metrics with distinct instrumentation scope
	if m.spanCountTotal != nil && m.spanBytesTotal != nil {
		m.spanCountTotal.Add(ctx, count, metric.WithAttributes(attrs...))
		m.spanBytesTotal.Add(ctx, bytes, metric.WithAttributes(attrs...))
	}
}

// AddLogMetrics records log count and bytes to OpenTelemetry metrics with distinct instrumentation scope
func (m *ExporterMetrics) AddLogMetrics(ctx context.Context, count int64, bytes int64, attrs []attribute.KeyValue) {
	// Record OpenTelemetry metrics with distinct instrumentation scope
	if m.logCountTotal != nil && m.logBytesTotal != nil {
		m.logCountTotal.Add(ctx, count, metric.WithAttributes(attrs...))
		m.logBytesTotal.Add(ctx, bytes, metric.WithAttributes(attrs...))
	}
}

// Note: OpenTelemetry counters are monotonic and cannot be reset.
// They accumulate values over the lifetime of the application.

// GetMetrics returns the metrics instance for the exporter
// Note: Individual metric values cannot be read from OpenTelemetry counters.
// Metrics are exported through the configured OpenTelemetry metrics pipeline.
func (e *enhanceIndexingS3Exporter) GetMetrics() *ExporterMetrics {
	return e.metrics
}
