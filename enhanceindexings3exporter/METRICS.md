# Metrics Export

The enhance-indexing-s3-exporter can export internal metrics about its operation using OpenTelemetry's OTLP/HTTP protocol. This allows you to monitor the exporter's performance and health.

## Configuration

To enable metrics export, add the `metrics` section to your exporter configuration:

```yaml
exporters:
  enhance_indexing_s3_exporter:
    # ... other configuration ...
    
    metrics:
      enabled: true                    # Enable or disable metrics export
      endpoint: localhost:4318         # OTLP/HTTP endpoint (host:port)
      insecure: true                   # Use HTTP instead of HTTPS
      export_interval_seconds: 10      # How often to export metrics
      headers:                         # Optional headers for authentication
        x-honeycomb-team: "your-key"
```

### Configuration Options

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | boolean | `false` | Enable or disable metrics export |
| `endpoint` | string | `localhost:4318` | OTLP/HTTP endpoint (hostname:port only, no path) |
| `insecure` | boolean | `true` | Use HTTP instead of HTTPS |
| `export_interval_seconds` | int | `10` | How often to export metrics in seconds |
| `headers` | map[string]string | `{}` | Additional HTTP headers to send with each export |

## Exported Metrics

The exporter provides the following metrics using **delta aggregation temporality**, which reports the change in value since the last export. This is the preferred temporality for many observability backends including Honeycomb.

### `indexer_spans_total`

**Type:** Counter  
**Unit:** Count  
**Description:** Total number of spans processed by the indexer

**Attributes:**
- `marshaler`: The marshaler type (otlp_json or otlp_protobuf)
- `api_endpoint`: The Honeycomb API endpoint being used

### `indexer_span_bytes_total`

**Type:** Counter  
**Unit:** Bytes  
**Description:** Total bytes of span data processed by the indexer

**Attributes:**
- `marshaler`: The marshaler type (otlp_json or otlp_protobuf)
- `api_endpoint`: The Honeycomb API endpoint being used

### `indexer_logs_total`

**Type:** Counter  
**Unit:** Count  
**Description:** Total number of log records processed by the indexer

**Attributes:**
- `marshaler`: The marshaler type (otlp_json or otlp_protobuf)
- `api_endpoint`: The Honeycomb API endpoint being used

### `indexer_log_bytes_total`

**Type:** Counter  
**Unit:** Bytes  
**Description:** Total bytes of log data processed by the indexer

**Attributes:**
- `marshaler`: The marshaler type (otlp_json or otlp_protobuf)
- `api_endpoint`: The Honeycomb API endpoint being used

## Example Configurations

### Send Metrics to Local OTLP Collector

```yaml
metrics:
  enabled: true
  endpoint: localhost:4318
  insecure: true
  export_interval_seconds: 10
```

### Send Metrics to Honeycomb

```yaml
metrics:
  enabled: true
  endpoint: api.honeycomb.io:443
  insecure: false
  export_interval_seconds: 30
  headers:
    x-honeycomb-team: ${HONEYCOMB_METRICS_API_KEY}
    x-honeycomb-dataset: exporter-metrics
```

### Send Metrics to Custom OTLP Backend with Authentication

```yaml
metrics:
  enabled: true
  endpoint: otlp.example.com:4318
  insecure: false
  export_interval_seconds: 15
  headers:
    Authorization: "Bearer ${OTLP_TOKEN}"
```

## Architecture

The metrics implementation uses OpenTelemetry's Go SDK with the following components:

1. **MetricsProvider**: Manages the lifecycle of the OpenTelemetry MeterProvider
   - Creates OTLP/HTTP exporter with configured endpoint
   - Sets up periodic metric export on the configured interval
   - Provides graceful shutdown with metric flushing

2. **ExporterMetrics**: Wraps the individual metric instruments
   - Uses a distinct instrumentation scope: `github.com/honeycombio/enhance-indexing-s3-exporter`
   - Pre-computes attributes to avoid allocations on each metric recording
   - Provides methods to record span and log metrics

3. **Delta Temporality**: All metrics use delta aggregation temporality
   - Reports the change in value since the last export
   - Preferred by backends like Honeycomb for accurate rate calculations
   - Configured via `WithTemporalitySelector` on the periodic reader

4. **Integration**: The metrics provider is initialized during exporter creation
   - If metrics are disabled, a no-op implementation is used
   - The metrics provider is shut down gracefully when the exporter stops

## Troubleshooting

### Metrics are not appearing in my backend

1. Check that `enabled: true` is set in the configuration
2. Verify the endpoint is correct (hostname:port format)
3. Check if the endpoint is reachable from your deployment
4. Look for error logs during exporter startup
5. Verify authentication headers if required by your backend

### High memory usage

If you notice high memory usage, consider:
- Increasing `export_interval_seconds` to reduce export frequency
- Checking if the OTLP endpoint is responsive (slow endpoints can cause buffering)

### Metrics provider initialization fails

Common causes:
- Invalid endpoint format (should be `hostname:port`, not a full URL)
- Network connectivity issues
- Incorrect TLS/SSL configuration (check `insecure` setting)

Check the exporter logs for detailed error messages during startup.

## Implementation Reference

The metrics implementation follows the [OpenTelemetry Go Metrics documentation](https://opentelemetry.io/docs/languages/go/instrumentation/#metrics), specifically:

- Using `metric.NewMeterProvider` with OTLP HTTP exporter
- Configuring periodic readers for automatic export
- Properly handling resource attributes and instrumentation scope
- Implementing graceful shutdown with metric flushing

