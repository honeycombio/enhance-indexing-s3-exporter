package enhanceindexings3exporter

import (
	"context"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

func TestExporterMetricsCollection(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	// Create test config
	config := &Config{
		S3Uploader: awss3exporter.S3UploaderConfig{
			Region:            "us-east-1",
			S3Bucket:          "test-bucket",
			S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			Compression:       "none",
		},
		MarshalerName: awss3exporter.OtlpJSON,
		APIEndpoint:   "http://localhost:8086",
		IndexedFields: []fieldName{"service.name"},
	}

	// Create mock S3 writer
	mockUploader := &mockS3Uploader{}
	s3Writer := NewS3Writer(&config.S3Uploader, config.MarshalerName, nil, logger)
	s3Writer.(*s3Writer).uploader = mockUploader

	// Create exporter
	indexManager := NewIndexManager(config, logger)
	exporter, err := newEnhanceIndexingS3Exporter(config, logger, indexManager)
	require.NoError(t, err)
	exporter.s3Writer = s3Writer

	// Verify metrics instance is created
	assert.NotNil(t, exporter.GetMetrics())
	assert.NotNil(t, exporter.GetMetrics().meter)
	assert.NotNil(t, exporter.GetMetrics().spanCountTotal)
	assert.NotNil(t, exporter.GetMetrics().spanBytesTotal)
	assert.NotNil(t, exporter.GetMetrics().logCountTotal)
	assert.NotNil(t, exporter.GetMetrics().logBytesTotal)

	// Create test traces with 2 spans
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")
	ss := rs.ScopeSpans().AppendEmpty()

	// Add first span
	span1 := ss.Spans().AppendEmpty()
	span1.SetName("test-span-1")
	span1.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
	span1.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 8})

	// Add second span
	span2 := ss.Spans().AppendEmpty()
	span2.SetName("test-span-2")
	span2.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 17})
	span2.SetSpanID([8]byte{1, 2, 3, 4, 5, 6, 7, 9})

	// Consume traces - this should record metrics to OpenTelemetry
	err = exporter.consumeTraces(ctx, traces)
	require.NoError(t, err)

	// Note: OpenTelemetry counters are write-only, so we can't verify the exact values
	// The metrics are recorded and will be exported through the configured pipeline

	// Create test logs with 3 log records
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "test-service")
	sl := rl.ScopeLogs().AppendEmpty()

	// Add first log record
	logRecord1 := sl.LogRecords().AppendEmpty()
	logRecord1.Body().SetStr("test log message 1")
	logRecord1.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	// Add second log record
	logRecord2 := sl.LogRecords().AppendEmpty()
	logRecord2.Body().SetStr("test log message 2")
	logRecord2.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	// Add third log record
	logRecord3 := sl.LogRecords().AppendEmpty()
	logRecord3.Body().SetStr("test log message 3")
	logRecord3.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	// Consume logs - this should record metrics to OpenTelemetry
	err = exporter.consumeLogs(ctx, logs)
	require.NoError(t, err)

	// Consume more traces and logs to test that metrics recording doesn't fail
	err = exporter.consumeTraces(ctx, traces)
	require.NoError(t, err)
	err = exporter.consumeLogs(ctx, logs)
	require.NoError(t, err)

	// Test direct metrics access - verify the metrics instance is accessible
	metrics := exporter.GetMetrics()
	assert.NotNil(t, metrics)
	assert.NotNil(t, metrics.meter)
	assert.NotNil(t, metrics.spanCountTotal)
	assert.NotNil(t, metrics.spanBytesTotal)
	assert.NotNil(t, metrics.logCountTotal)
	assert.NotNil(t, metrics.logBytesTotal)
}

func TestExporterMetricsThreadSafety(t *testing.T) {
	// Test that metrics creation and concurrent recording works without panics
	metrics, err := NewExporterMetrics()
	require.NoError(t, err)

	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("test", "value"),
	}

	// Run concurrent metric updates
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			metrics.AddSpanMetrics(ctx, 1, 100, attrs)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			metrics.AddLogMetrics(ctx, 1, 200, attrs)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify metrics instance is still valid after concurrent operations
	assert.NotNil(t, metrics.meter)
	assert.NotNil(t, metrics.spanCountTotal)
	assert.NotNil(t, metrics.spanBytesTotal)
	assert.NotNil(t, metrics.logCountTotal)
	assert.NotNil(t, metrics.logBytesTotal)
}
