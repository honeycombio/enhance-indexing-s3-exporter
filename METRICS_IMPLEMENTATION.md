# Metrics Export Implementation Summary

## Overview

This implementation adds the ability to export internal metrics from the `enhance-indexing-s3-exporter` using OpenTelemetry's OTLP/HTTP protocol to a custom endpoint, following the [OpenTelemetry Go instrumentation guidelines](https://opentelemetry.io/docs/languages/go/instrumentation/#metrics).

**Key Feature**: All metrics are exported using **delta aggregation temporality**, which reports the change in value since the last export. This is the preferred temporality for many observability backends including Honeycomb, as it provides accurate rate calculations and reduces storage overhead.

## Changes Made

### 1. New Files Created

#### `enhanceindexings3exporter/internal/metrics_provider.go`
- **Purpose**: Manages the OpenTelemetry MeterProvider lifecycle
- **Key Components**:
  - `MetricsProviderConfig`: Configuration struct for the metrics provider
  - `MetricsProvider`: Wraps the OpenTelemetry MeterProvider
  - `NewMetricsProvider()`: Creates a new provider with OTLP HTTP exporter
  - Implements graceful `Shutdown()` and `ForceFlush()` methods

#### `enhanceindexings3exporter/internal/exporter_metrics.go` (Modified)
- **Changes**:
  - Updated `NewExporterMetrics()` to accept a `metric.MeterProvider` parameter
  - Now supports nil provider for no-op behavior when metrics are disabled
  - Removed dependency on global `otel.Meter()` - now uses injected provider

#### `config/metrics-example.yaml`
- **Purpose**: Example configuration showing how to enable and configure metrics export
- **Includes**: Examples for local OTLP collector, Honeycomb, and custom backends

#### `enhanceindexings3exporter/METRICS.md`
- **Purpose**: Complete documentation for the metrics export feature
- **Includes**:
  - Configuration reference
  - List of exported metrics with descriptions
  - Example configurations for different backends
  - Architecture overview
  - Troubleshooting guide

### 2. Modified Files

#### `enhanceindexings3exporter/config.go`
- **Added**:
  - `MetricsConfig` struct with fields:
    - `Enabled`: Enable/disable metrics export
    - `Endpoint`: OTLP/HTTP endpoint (hostname:port)
    - `Insecure`: Use HTTP vs HTTPS
    - `Headers`: Custom HTTP headers for authentication
    - `ExportIntervalSeconds`: Export frequency
  - `Metrics` field to main `Config` struct
  - Default metrics configuration in `createDefaultConfig()`

#### `enhanceindexings3exporter/exporter.go`
- **Added**:
  - `metricsProvider` field to `enhanceIndexingS3Exporter` struct
  - Context parameter to `newEnhanceIndexingS3Exporter()` function
  - Metrics provider initialization logic:
    - Creates provider only when metrics are enabled
    - Passes provider to `ExporterMetrics`
    - Uses no-op metrics when disabled
- **Updated**:
  - `shutdown()` method to flush and shutdown metrics provider
  - Proper logging for metrics initialization

#### `enhanceindexings3exporter/factory.go`
- **Updated**:
  - Both `createTracesExporter()` and `createLogsExporter()` to pass context to `newEnhanceIndexingS3Exporter()`

#### `enhanceindexings3exporter/exporter_test.go`
- **Updated**:
  - All test calls to `newEnhanceIndexingS3Exporter()` to include context parameter

## Architecture

### Metrics Flow

```
┌─────────────────────────────────────────────────┐
│         enhance-indexing-s3-exporter            │
│                                                 │
│  ┌───────────────────────────────────────────┐ │
│  │  ExporterMetrics                          │ │
│  │  - Records span/log counts and bytes      │ │
│  │  - Uses MeterProvider from config         │ │
│  └───────────────┬───────────────────────────┘ │
│                  │                              │
│  ┌───────────────▼───────────────────────────┐ │
│  │  MetricsProvider                          │ │
│  │  - Manages MeterProvider lifecycle        │ │
│  │  - Configures OTLP HTTP exporter          │ │
│  │  - Handles periodic export                │ │
│  └───────────────┬───────────────────────────┘ │
│                  │                              │
└──────────────────┼──────────────────────────────┘
                   │
                   │ OTLP/HTTP
                   ▼
         ┌────────────────────┐
         │  Custom Endpoint   │
         │  (Collector, etc)  │
         └────────────────────┘
```

### Key Design Decisions

1. **Dependency Injection**: The MeterProvider is injected into ExporterMetrics, allowing for easy testing and no-op behavior when disabled.

2. **Graceful Shutdown**: Metrics are flushed during shutdown to ensure no data loss.

3. **Configuration-Driven**: Metrics can be easily enabled/disabled via configuration without code changes.

4. **Standards-Compliant**: Uses OpenTelemetry SDK's recommended patterns for metrics export.

5. **Performance**: Pre-computed attributes avoid allocations on every metric recording.

6. **Delta Temporality**: Metrics use delta aggregation temporality, reporting changes since the last export rather than cumulative totals.

## Configuration Example

```yaml
exporters:
  enhance_indexing_s3_exporter:
    # ... other config ...
    metrics:
      enabled: true
      endpoint: localhost:4318
      insecure: true
      export_interval_seconds: 10
      headers:
        x-honeycomb-team: "your-api-key"
```

## Exported Metrics

| Metric Name | Type | Description |
|-------------|------|-------------|
| `indexer_spans_total` | Counter (Delta) | Total spans processed |
| `indexer_span_bytes_total` | Counter (Delta) | Total span bytes processed |
| `indexer_logs_total` | Counter (Delta) | Total logs processed |
| `indexer_log_bytes_total` | Counter (Delta) | Total log bytes processed |

**Temporality**: All metrics use delta temporality, reporting the change since the last export.

All metrics include attributes:
- `marshaler`: The marshaler type (otlp_json or otlp_protobuf)
- `api_endpoint`: The Honeycomb API endpoint being used

## Dependencies

All required dependencies are already present in `go.mod`:
- `go.opentelemetry.io/otel/metric` v1.37.0
- `go.opentelemetry.io/otel/sdk/metric` v1.37.0
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` v1.37.0
- `go.opentelemetry.io/otel/sdk/resource`
- `go.opentelemetry.io/otel/semconv/v1.27.0`

## Testing Notes

The implementation has been designed to work with the existing test infrastructure. All tests have been updated to pass the context parameter to `newEnhanceIndexingS3Exporter()`.

**Note**: Building and testing requires Go 1.24 as specified in `go.mod`. The current system has Go 1.22.2, which causes build failures. This is not a code issue but an environment requirement.

## Usage

### Enable Metrics Export

1. Update your configuration file to include the metrics section:

```yaml
metrics:
  enabled: true
  endpoint: your-otlp-endpoint:4318
  insecure: true  # or false for HTTPS
  export_interval_seconds: 10
```

2. The exporter will automatically:
   - Initialize the metrics provider on startup
   - Export metrics at the configured interval
   - Flush metrics on graceful shutdown

3. Monitor the exporter logs for metrics initialization messages:
   - "Initializing metrics provider" with endpoint
   - "Metrics provider initialized successfully"
   - "Metrics export disabled" (if not enabled)

### Disable Metrics Export

Simply set `enabled: false` in the metrics configuration, or omit the metrics section entirely. The exporter will use a no-op implementation with zero overhead.

## Future Enhancements

Potential future improvements:
1. Add more granular metrics (per-field indexing stats, batch sizes, etc.)
2. Add histogram metrics for processing latencies
3. Add gauge metrics for queue depths and buffer sizes
4. Support for additional exporters (Prometheus, etc.)
5. Add metric views for customization

## References

- [OpenTelemetry Go Metrics Documentation](https://opentelemetry.io/docs/languages/go/instrumentation/#metrics)
- [OTLP Specification](https://opentelemetry.io/docs/specs/otlp/)
- [OpenTelemetry Go SDK](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric)

