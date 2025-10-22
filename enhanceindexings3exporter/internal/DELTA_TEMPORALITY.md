# Delta Temporality in Metrics

## Overview

This metrics implementation uses **delta aggregation temporality** for all exported metrics. This document explains what that means and why it's important.

## What is Delta Temporality?

In OpenTelemetry, temporality determines how metric values are reported:

### Cumulative Temporality
- Reports the total accumulated value since the application started
- Example: If 100 spans were processed in minute 1 and 50 in minute 2, exports would be:
  - Minute 1: 100
  - Minute 2: 150 (cumulative total)

### Delta Temporality (Used Here)
- Reports only the change since the last export
- Example: Same scenario would export:
  - Minute 1: 100
  - Minute 2: 50 (delta since last export)

## Why Delta Temporality?

### 1. Accurate Rate Calculations
Backends like Honeycomb can accurately calculate rates (e.g., spans/second) from delta values without needing to compute differences between cumulative totals.

### 2. Reduced Storage
Delta values tend to be smaller and compress better, especially for long-running services.

### 3. Handles Restarts Better
When a service restarts, cumulative metrics reset to zero, which can cause confusion. Delta metrics naturally handle restarts.

### 4. Backend Preference
Many modern observability backends (including Honeycomb) prefer or require delta temporality for correct metric interpretation.

## Implementation

The delta temporality is configured in `metrics_provider.go`:

```go
// deltaTemporalitySelector configures the reader to use delta temporality for all instruments.
func deltaTemporalitySelector(kind metric.InstrumentKind) metricdata.Temporality {
	return metricdata.DeltaTemporality
}

// Applied to the periodic reader:
reader := metric.NewPeriodicReader(
	exporter,
	metric.WithInterval(config.ExportInterval),
	metric.WithTemporalitySelector(deltaTemporalitySelector),
)
```

## Example

Consider the `indexer_spans_total` counter:

**Scenario**: Your exporter processes:
- Time 0-10s: 1000 spans
- Time 10-20s: 1500 spans
- Time 20-30s: 800 spans

**With Delta Temporality (our implementation)**:
```
Export at 10s: indexer_spans_total = 1000
Export at 20s: indexer_spans_total = 1500
Export at 30s: indexer_spans_total = 800
```

Each value represents spans processed *in that interval*.

**With Cumulative Temporality (not used)**:
```
Export at 10s: indexer_spans_total = 1000
Export at 20s: indexer_spans_total = 2500  (1000 + 1500)
Export at 30s: indexer_spans_total = 3300  (1000 + 1500 + 800)
```

Values continuously grow, requiring the backend to compute deltas for rate calculations.

## Metric Types and Temporality

All our counter metrics use delta temporality:
- `indexer_spans_total` (Counter)
- `indexer_span_bytes_total` (Counter)
- `indexer_logs_total` (Counter)
- `indexer_log_bytes_total` (Counter)

Note: For histogram and gauge metrics (if added in the future), different temporality considerations apply:
- **Histograms**: Usually delta for event counts, cumulative for value distributions
- **Gauges**: Point-in-time values, temporality doesn't apply

## Verification

To verify delta temporality is working:

1. Enable metrics export to a backend that shows temporality (like Honeycomb)
2. Generate a known amount of traffic
3. Check that metric values show the delta (change) between exports, not cumulative totals

Example query in Honeycomb:
```
SUM(indexer_spans_total)
```

With a consistent load of 100 spans/second and 10-second export interval, you should see values around 1000 for each data point, not increasing cumulative values.

## References

- [OpenTelemetry Metrics Data Model](https://opentelemetry.io/docs/specs/otel/metrics/data-model/#temporality)
- [Honeycomb's Temporality Preference](https://docs.honeycomb.io/send-data/opentelemetry/metrics/)
- [Go SDK Temporality Configuration](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric#WithTemporalitySelector)

