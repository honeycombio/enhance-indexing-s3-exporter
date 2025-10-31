package enhanceindexings3exporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

func TestRecordTracesUsage(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		count int64
	}{
		{
			name:  "Traces with spans",
			bytes: 1000,
			count: 5,
		},
		{
			name:  "Empty traces",
			bytes: 0,
			count: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := &enhanceIndexingS3Exporter{
				logger: zap.NewNop(),
			}

			// Record usage
			exporter.RecordTracesUsage(tt.bytes, tt.count)

			// Verify count matches expected
			assert.Equal(t, tt.count, exporter.usageTraces.count)

			// Verify bytes matches expected
			assert.Equal(t, tt.bytes, exporter.usageTraces.bytes)
		})
	}
}

func TestRecordLogsUsage(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		count int64
	}{
		{
			name:  "Logs with records",
			bytes: 500,
			count: 5,
		},
		{
			name:  "Empty logs",
			bytes: 0,
			count: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter := &enhanceIndexingS3Exporter{
				logger: zap.NewNop(),
			}

			// Record usage
			exporter.RecordLogsUsage(tt.bytes, tt.count)

			// Verify count matches expected
			assert.Equal(t, tt.count, exporter.usageLogs.count)

			// Verify bytes matches expected
			assert.Equal(t, tt.bytes, exporter.usageLogs.bytes)
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
			expectedMetricCount: 0,
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
			requestReceived := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestReceived = true

				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/2/teams/"+tt.teamSlug+"/enhance_indexer_usage", r.URL.Path)

				assert.Equal(t, "application/vnd.api+json", r.Header.Get("Content-Type"))
				assert.Contains(t, r.Header.Get("Authorization"), "Bearer")

				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					_, _ = w.Write([]byte(tt.responseBody))
				}
			}))
			defer server.Close()

			exporter := &enhanceIndexingS3Exporter{
				config: &Config{
					APIEndpoint: server.URL,
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

			// Verify usage was reset if metrics were created
			if tt.tracesBytes > 0 || tt.tracesCount > 0 {
				assert.Equal(t, int64(0), exporter.usageTraces.bytes)
				assert.Equal(t, int64(0), exporter.usageTraces.count)
			}

			// Verify request was sent (or not) based on conditions
			shouldSendRequest := tt.tracesBytes > 0 && tt.teamSlug != ""
			assert.Equal(t, shouldSendRequest, requestReceived, "request sent mismatch")
		})
	}
}
