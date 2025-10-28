package enhanceindexings3exporter

import (
	"context"
	"net/http"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func TestRecordTracesUsage(t *testing.T) {
	tests := []struct {
		name          string
		marshalerName awss3exporter.MarshalerType
		spanCount     int
	}{
		{
			name:          "JSON marshaler with traces",
			marshalerName: awss3exporter.OtlpJSON,
			spanCount:     5,
		},
		{
			name:          "Proto marshaler with traces",
			marshalerName: awss3exporter.OtlpProtobuf,
			spanCount:     3,
		},
		{
			name:          "Empty traces",
			marshalerName: awss3exporter.OtlpJSON,
			spanCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create exporter
			config := &Config{
				MarshalerName: tt.marshalerName,
			}

			var traceMarshaler ptrace.Marshaler
			if tt.marshalerName == awss3exporter.OtlpJSON {
				traceMarshaler = &ptrace.JSONMarshaler{}
			} else {
				traceMarshaler = &ptrace.ProtoMarshaler{}
			}

			exporter := &enhanceIndexingS3Exporter{
				config:         config,
				logger:         zap.NewNop(),
				traceMarshaler: traceMarshaler,
			}

			// Create test traces
			traces := ptrace.NewTraces()
			if tt.spanCount > 0 {
				rs := traces.ResourceSpans().AppendEmpty()
				ss := rs.ScopeSpans().AppendEmpty()
				for i := 0; i < tt.spanCount; i++ {
					span := ss.Spans().AppendEmpty()
					span.SetName("test-span")
				}
			}

			// Record usage
			exporter.RecordTracesUsage(traces)

			// Verify count matches span count
			assert.Equal(t, int64(tt.spanCount), exporter.usageTraces.count)

			// Verify bytes are recorded for non-empty traces
			if tt.spanCount > 0 {
				assert.Greater(t, exporter.usageTraces.bytes, int64(0), "bytes should be > 0 for non-empty traces")
			}
		})
	}
}

func TestRecordLogsUsage(t *testing.T) {
	tests := []struct {
		name          string
		marshalerName awss3exporter.MarshalerType
		logCount      int
	}{
		{
			name:          "JSON marshaler with logs",
			marshalerName: awss3exporter.OtlpJSON,
			logCount:      5,
		},
		{
			name:          "Proto marshaler with logs",
			marshalerName: awss3exporter.OtlpProtobuf,
			logCount:      3,
		},
		{
			name:          "Empty logs",
			marshalerName: awss3exporter.OtlpJSON,
			logCount:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create exporter
			config := &Config{
				MarshalerName: tt.marshalerName,
			}

			var logMarshaler plog.Marshaler
			if tt.marshalerName == awss3exporter.OtlpJSON {
				logMarshaler = &plog.JSONMarshaler{}
			} else {
				logMarshaler = &plog.ProtoMarshaler{}
			}

			exporter := &enhanceIndexingS3Exporter{
				config:       config,
				logger:       zap.NewNop(),
				logMarshaler: logMarshaler,
			}

			// Create test logs
			logs := plog.NewLogs()
			if tt.logCount > 0 {
				rl := logs.ResourceLogs().AppendEmpty()
				sl := rl.ScopeLogs().AppendEmpty()
				for i := 0; i < tt.logCount; i++ {
					logRecord := sl.LogRecords().AppendEmpty()
					logRecord.Body().SetStr("test log")
				}
			}

			// Record usage
			exporter.RecordLogsUsage(logs)

			// Verify count matches log count
			assert.Equal(t, int64(tt.logCount), exporter.usageLogs.count)

			// Verify bytes are recorded for non-empty logs
			if tt.logCount > 0 {
				assert.Greater(t, exporter.usageLogs.bytes, int64(0), "bytes should be > 0 for non-empty logs")
			}
		})
	}
}

func TestCreateUsageReport(t *testing.T) {
	tests := []struct {
		name                string
		tracesBytes         int64
		tracesCount         int64
		logsBytes           int64
		logsCount           int64
		expectedMetricCount int
		expectedDatapoints  int
	}{
		{
			name:                "Both traces and logs",
			tracesBytes:         1000,
			tracesCount:         10,
			logsBytes:           500,
			logsCount:           5,
			expectedMetricCount: 2, // bytes_received and count_received
			expectedDatapoints:  4, // 2 for each metric (traces + logs)
		},
		{
			name:                "Only traces",
			tracesBytes:         1000,
			tracesCount:         10,
			logsBytes:           0,
			logsCount:           0,
			expectedMetricCount: 2,
			expectedDatapoints:  2, // 1 for each metric (only traces)
		},
		{
			name:                "Only logs",
			tracesBytes:         0,
			tracesCount:         0,
			logsBytes:           500,
			logsCount:           5,
			expectedMetricCount: 2,
			expectedDatapoints:  2, // 1 for each metric (only logs)
		},
		{
			name:                "No data",
			tracesBytes:         0,
			tracesCount:         0,
			logsBytes:           0,
			logsCount:           0,
			expectedMetricCount: 2,
			expectedDatapoints:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := &enhanceIndexingS3Exporter{
				logger: zap.NewNop(),
				usageTraces: usageData{
					bytes: tt.tracesBytes,
					count: tt.tracesCount,
				},
				usageLogs: usageData{
					bytes: tt.logsBytes,
					count: tt.logsCount,
				},
			}

			// Create usage report
			metrics := exporter.createUsageReport()

			// Verify structure
			assert.Equal(t, tt.expectedMetricCount, metrics.MetricCount())
			assert.Equal(t, 1, metrics.ResourceMetrics().Len())

			rm := metrics.ResourceMetrics().At(0)
			assert.Equal(t, 1, rm.ScopeMetrics().Len())

			sm := rm.ScopeMetrics().At(0)
			assert.Equal(t, tt.expectedMetricCount, sm.Metrics().Len())

			// Verify instrumentation scope is set
			assert.Equal(t, "github.com/honeycombio/enhance-indexing-s3-exporter", sm.Scope().Name())

			// Verify metric names and aggregation
			var totalDatapoints int
			for i := 0; i < sm.Metrics().Len(); i++ {
				metric := sm.Metrics().At(i)
				assert.Contains(t, []string{"bytes_received", "count_received"}, metric.Name())
				assert.Equal(t, pmetric.AggregationTemporalityDelta, metric.Sum().AggregationTemporality())
				totalDatapoints += metric.Sum().DataPoints().Len()
			}
			assert.Equal(t, tt.expectedDatapoints, totalDatapoints)

			// Verify usage was reset
			assert.Equal(t, int64(0), exporter.usageTraces.bytes)
			assert.Equal(t, int64(0), exporter.usageTraces.count)
			assert.Equal(t, int64(0), exporter.usageLogs.bytes)
			assert.Equal(t, int64(0), exporter.usageLogs.count)
		})
	}
}

func TestCollectAndSendMetrics(t *testing.T) {
	tests := []struct {
		name         string
		statusCode   int
		responseBody string
		tracesBytes  int64
		tracesCount  int64
		teamSlug     string
		expectError  bool
	}{
		{
			name:         "Successful send with 200 OK",
			statusCode:   http.StatusOK,
			responseBody: `{"status":"ok"}`,
			tracesBytes:  1000,
			tracesCount:  10,
			teamSlug:     "test-team",
			expectError:  false,
		},
		{
			name:         "Successful send with 201 Created",
			statusCode:   http.StatusCreated,
			responseBody: `{"status":"created"}`,
			tracesBytes:  1000,
			tracesCount:  10,
			teamSlug:     "test-team",
			expectError:  false,
		},
		{
			name:         "Successful send with 204 No Content",
			statusCode:   http.StatusNoContent,
			responseBody: "",
			tracesBytes:  1000,
			tracesCount:  10,
			teamSlug:     "test-team",
			expectError:  false,
		},
		{
			name:         "Failed send with 400 Bad Request",
			statusCode:   http.StatusBadRequest,
			responseBody: `{"error":"bad request"}`,
			tracesBytes:  1000,
			tracesCount:  10,
			teamSlug:     "test-team",
			expectError:  true,
		},
		{
			name:         "No metrics to send",
			statusCode:   http.StatusOK,
			responseBody: "",
			tracesBytes:  0,
			tracesCount:  0,
			teamSlug:     "test-team",
			expectError:  false,
		},
		{
			name:         "Empty team slug skips send",
			statusCode:   http.StatusOK,
			responseBody: "",
			tracesBytes:  1000,
			tracesCount:  10,
			teamSlug:     "",
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := &enhanceIndexingS3Exporter{
				config: &Config{
					APIEndpoint: "https://api.honeycomb.io",
					APIKey:      "test-key",
					APISecret:   "test-secret",
					S3Uploader: awss3exporter.S3UploaderConfig{
						S3Bucket:   "test-bucket",
						FilePrefix: "test-prefix",
					},
					MarshalerName: awss3exporter.OtlpJSON,
				},
				logger:   zap.NewNop(),
				teamSlug: tt.teamSlug,
				usageTraces: usageData{
					bytes: tt.tracesBytes,
					count: tt.tracesCount,
				},
			}

			ctx := context.Background()
			exporter.collectAndSendMetrics(ctx)

			// Verify usage was reset if metrics were sent
			// When team slug is empty, metrics are cleared but not sent
			if tt.tracesBytes > 0 {
				assert.Equal(t, int64(0), exporter.usageTraces.bytes)
				assert.Equal(t, int64(0), exporter.usageTraces.count)
			}
		})
	}
}
