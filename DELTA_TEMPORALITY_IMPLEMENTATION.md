# Delta Temporality Implementation Summary

## What Was Implemented

The metrics export feature has been configured to use **delta aggregation temporality** for all exported metrics. This means that each metric export reports only the change in value since the last export, rather than cumulative totals.

## Code Changes

### File Modified: `enhanceindexings3exporter/internal/metrics_provider.go`

#### 1. Added Import
```go
import (
    // ... existing imports
    "go.opentelemetry.io/otel/sdk/metric/metricdata"
)
```

#### 2. Added Delta Temporality Selector Function
```go
// deltaTemporalitySelector configures the reader to use delta temporality for all instruments.
// Delta temporality reports the change in value since the last export, which is preferred for
// many observability backends including Honeycomb.
func deltaTemporalitySelector(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.DeltaTemporality
}
```

#### 3. Applied Selector to Periodic Reader
```go
// Create periodic reader with configured interval and delta temporality
reader := metric.NewPeriodicReader(
	exporter,
	metric.WithInterval(config.ExportInterval),
	// Configure temporality selector to prefer delta for all instruments
	metric.WithTemporalitySelector(deltaTemporalitySelector),
)
```

## Documentation Updates

Updated the following files to document delta temporality:

1. **`enhanceindexings3exporter/METRICS.md`**
   - Added explanation that metrics use delta temporality
   - Updated metric descriptions
   - Added delta temporality to architecture section

2. **`METRICS_IMPLEMENTATION.md`**
   - Added key feature callout about delta temporality
   - Updated metric table to show "(Delta)" type
   - Added temporality note to design decisions

3. **`enhanceindexings3exporter/README.md`**
   - Updated exported metrics section with delta temporality note
   - Added explanation of why delta is preferred

4. **Created `enhanceindexings3exporter/internal/DELTA_TEMPORALITY.md`**
   - Comprehensive explanation of delta vs cumulative temporality
   - Examples showing the difference
   - Implementation details
   - Verification instructions

## How Delta Temporality Works

### Example Scenario

If your exporter processes spans continuously:

**Time Window 1 (0-10s)**: 1,000 spans processed
**Time Window 2 (10-20s)**: 1,500 spans processed  
**Time Window 3 (20-30s)**: 800 spans processed

### Exported Values (Delta Temporality - Our Implementation)

```
Export at 10s:  indexer_spans_total = 1,000
Export at 20s:  indexer_spans_total = 1,500
Export at 30s:  indexer_spans_total = 800
```

Each export shows spans processed **in that specific interval**.

### What Cumulative Would Look Like (Not Used)

```
Export at 10s:  indexer_spans_total = 1,000
Export at 20s:  indexer_spans_total = 2,500  (1,000 + 1,500)
Export at 30s:  indexer_spans_total = 3,300  (1,000 + 1,500 + 800)
```

Values continuously accumulate.

## Benefits of Delta Temporality

### 1. Accurate Rate Calculations
Backends can directly use delta values to calculate rates without computing differences:
- `rate = delta_value / time_interval`

### 2. Better Storage Efficiency
- Smaller numeric values compress better
- No need to store and compute cumulative differences

### 3. Handles Service Restarts
- When a service restarts, there's no confusing reset to zero
- Each export period stands alone

### 4. Backend Compatibility
- Honeycomb and many modern observability platforms prefer delta temporality
- Provides more accurate metric interpretation

## Affected Metrics

All four exported counter metrics use delta temporality:

| Metric | Type | Temporality |
|--------|------|-------------|
| `indexer_spans_total` | Counter | Delta |
| `indexer_span_bytes_total` | Counter | Delta |
| `indexer_logs_total` | Counter | Delta |
| `indexer_log_bytes_total` | Counter | Delta |

## Verification

To verify delta temporality is working correctly:

1. **Enable metrics export** in your configuration:
```yaml
metrics:
  enabled: true
  endpoint: your-backend:4318
  export_interval_seconds: 10
```

2. **Generate consistent load**: Send a steady stream of spans/logs to the exporter

3. **Check metric values**: 
   - In a dashboard or query interface
   - Values should show deltas (changes per interval)
   - NOT continuously increasing cumulative totals

4. **Expected pattern**: With consistent load, you should see relatively stable values (with natural variance), not ever-increasing numbers

### Example Honeycomb Query
```
SUM(indexer_spans_total)
```

With 100 spans/second and 10-second exports, you should see values around 1,000 for each data point.

## Configuration

No configuration changes are needed. Delta temporality is automatically applied to all metrics. It works with any OTLP backend:

```yaml
metrics:
  enabled: true
  endpoint: localhost:4318  # or any OTLP endpoint
  insecure: true
  export_interval_seconds: 10
```

## Technical Details

### OpenTelemetry SDK Configuration

The implementation uses the OpenTelemetry Go SDK's `WithTemporalitySelector` option:

```go
metric.NewPeriodicReader(
    exporter,
    metric.WithInterval(config.ExportInterval),
    metric.WithTemporalitySelector(deltaTemporalitySelector),
)
```

### Temporality Selector Function

The selector function is called for each metric instrument type and returns the desired temporality:

```go
func deltaTemporalitySelector(kind metric.InstrumentKind) metricdata.Temporality {
    return metricdata.DeltaTemporality
}
```

This applies to all instrument kinds (Counter, UpDownCounter, Histogram, etc.), though currently only Counter instruments are used.

## Future Considerations

If additional metric types are added in the future:

- **Histograms**: Consider delta for counts, but be aware of distribution requirements
- **Gauges**: Temporality doesn't apply (they represent point-in-time values)
- **UpDownCounters**: Delta typically makes sense for these as well

## References

- [OpenTelemetry Metrics Data Model - Temporality](https://opentelemetry.io/docs/specs/otel/metrics/data-model/#temporality)
- [Go SDK Metric Documentation](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric)
- [OTLP Exporter Configuration](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp)

## Testing

The implementation has been designed with delta temporality from the start. To test:

1. Build the exporter (requires Go 1.24)
2. Configure metrics export to a test OTLP collector
3. Generate test traffic
4. Verify metric values show deltas, not cumulative totals

Example test setup is provided in:
- `examples/docker-compose-with-metrics.yaml`
- `examples/otel-collector-metrics-config.yaml`
- `examples/exporter-config-with-metrics.yaml`

