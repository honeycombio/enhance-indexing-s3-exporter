package enhanceindexings3exporter

import "sync/atomic"

// ExporterMetrics holds metrics for the exporter
type ExporterMetrics struct {
	spanCount int64
	spanBytes int64
	logCount  int64
	logBytes  int64
}

// GetSpanCount returns the current span count
func (m *ExporterMetrics) GetSpanCount() int64 {
	return atomic.LoadInt64(&m.spanCount)
}

// GetSpanBytes returns the current span bytes
func (m *ExporterMetrics) GetSpanBytes() int64 {
	return atomic.LoadInt64(&m.spanBytes)
}

// GetLogCount returns the current log count
func (m *ExporterMetrics) GetLogCount() int64 {
	return atomic.LoadInt64(&m.logCount)
}

// GetLogBytes returns the current log bytes
func (m *ExporterMetrics) GetLogBytes() int64 {
	return atomic.LoadInt64(&m.logBytes)
}

// AddSpanMetrics atomically adds to span count and bytes
func (m *ExporterMetrics) AddSpanMetrics(count int64, bytes int64) {
	atomic.AddInt64(&m.spanCount, count)
	atomic.AddInt64(&m.spanBytes, bytes)
}

// AddLogMetrics atomically adds to log count and bytes
func (m *ExporterMetrics) AddLogMetrics(count int64, bytes int64) {
	atomic.AddInt64(&m.logCount, count)
	atomic.AddInt64(&m.logBytes, bytes)
}

// Reset resets all metrics to zero
func (m *ExporterMetrics) Reset() {
	atomic.StoreInt64(&m.spanCount, 0)
	atomic.StoreInt64(&m.spanBytes, 0)
	atomic.StoreInt64(&m.logCount, 0)
	atomic.StoreInt64(&m.logBytes, 0)
}

// GetMetrics returns the current metrics for the exporter
func (e *enhanceIndexingS3Exporter) GetMetrics() *ExporterMetrics {
	return e.metrics
}

// GetSpanCount returns the current span count
func (e *enhanceIndexingS3Exporter) GetSpanCount() int64 {
	return e.metrics.GetSpanCount()
}

// GetSpanBytes returns the current span bytes
func (e *enhanceIndexingS3Exporter) GetSpanBytes() int64 {
	return e.metrics.GetSpanBytes()
}

// GetLogCount returns the current log count
func (e *enhanceIndexingS3Exporter) GetLogCount() int64 {
	return e.metrics.GetLogCount()
}

// GetLogBytes returns the current log bytes
func (e *enhanceIndexingS3Exporter) GetLogBytes() int64 {
	return e.metrics.GetLogBytes()
}

// ResetMetrics resets all metrics to zero
func (e *enhanceIndexingS3Exporter) ResetMetrics() {
	e.metrics.Reset()
}
