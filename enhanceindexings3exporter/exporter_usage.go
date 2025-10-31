package enhanceindexings3exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

const (
	// instrumentationScopeName is the distinct scope name for indexer metrics
	instrumentationScopeName = "github.com/honeycombio/enhance-indexing-s3-exporter"
)

var (
	pmetricJSONMarshaller = pmetric.JSONMarshaler{}
)

// usageData tracks bytes and count for a signal type
type usageData struct {
	bytes int64
	count int64
}

// usageMetrics wraps pmetric.Metrics and implements custom JSON marshaling
type usageMetrics struct {
	Metrics pmetric.Metrics
}

func (u *usageMetrics) MarshalJSON() ([]byte, error) {
	return pmetricJSONMarshaller.MarshalMetrics(u.Metrics)
}

// createEnhanceIndexerUsageRecordAttributes represents the attributes for the usage record
type createEnhanceIndexerUsageRecordAttributes struct {
	S3Bucket     string        `json:"s3Bucket"`
	S3FilePrefix string        `json:"s3FilePrefix"`
	UsageData    *usageMetrics `json:"usageData"`
}

// enhanceIndexerUsageRecordContents represents the data content for the request
type enhanceIndexerUsageRecordContents struct {
	Type       string                                    `json:"type"`
	ID         string                                    `json:"id"`
	Attributes createEnhanceIndexerUsageRecordAttributes `json:"attributes"`
}

// enhanceIndexerUsageRecordRequest represents the full JSONAPI request
type enhanceIndexerUsageRecordRequest struct {
	Data enhanceIndexerUsageRecordContents `json:"data"`
}

// RecordTracesUsage records traces usage for metrics reporting
func (e *enhanceIndexingS3Exporter) RecordTracesUsage(bytes int64, count int64) {
	if bytes == 0 {
		return
	}

	e.usageTracesMutex.Lock()
	e.usageTraces.bytes += bytes
	e.usageTraces.count += count
	e.usageTracesMutex.Unlock()
}

// RecordLogsUsage records logs usage for metrics reporting
func (e *enhanceIndexingS3Exporter) RecordLogsUsage(bytes int64, count int64) {
	if bytes == 0 {
		return
	}

	e.usageLogsMutex.Lock()
	e.usageLogs.bytes += bytes
	e.usageLogs.count += count
	e.usageLogsMutex.Unlock()
}

// createUsageReport creates a pmetric.Metrics report from the current usage data
// and resets the usage counters
func (e *enhanceIndexingS3Exporter) createUsageReport() pmetric.Metrics {
	// Copy current usage and reset
	e.usageTracesMutex.Lock()
	tracesBytes := e.usageTraces.bytes
	tracesCount := e.usageTraces.count
	e.usageTraces = usageData{}
	e.usageTracesMutex.Unlock()

	e.usageLogsMutex.Lock()
	logsBytes := e.usageLogs.bytes
	logsCount := e.usageLogs.count
	e.usageLogs = usageData{}
	e.usageLogsMutex.Unlock()

	// Build pmetric.Metrics
	m := pmetric.NewMetrics()
	rm := m.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName(instrumentationScopeName)

	// Only create bytes_received metric if we have data
	if tracesBytes > 0 || logsBytes > 0 {
		bytesMetric := sm.Metrics().AppendEmpty()
		bytesMetric.SetName("bytes_received")
		bytesSum := bytesMetric.SetEmptySum()
		bytesSum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

		if tracesBytes > 0 {
			dp := bytesSum.DataPoints().AppendEmpty()
			dp.SetIntValue(tracesBytes)
			dp.Attributes().PutStr("signal", "traces")
			dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		}

		if logsBytes > 0 {
			dp := bytesSum.DataPoints().AppendEmpty()
			dp.SetIntValue(logsBytes)
			dp.Attributes().PutStr("signal", "logs")
			dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		}
	}

	// Only create count_received metric if we have data
	if tracesCount > 0 || logsCount > 0 {
		countMetric := sm.Metrics().AppendEmpty()
		countMetric.SetName("count_received")
		countSum := countMetric.SetEmptySum()
		countSum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

		if tracesCount > 0 {
			dp := countSum.DataPoints().AppendEmpty()
			dp.SetIntValue(tracesCount)
			dp.Attributes().PutStr("signal", "traces")
			dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		}

		if logsCount > 0 {
			dp := countSum.DataPoints().AppendEmpty()
			dp.SetIntValue(logsCount)
			dp.Attributes().PutStr("signal", "logs")
			dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		}
	}

	return m
}

// startMetricsCollection collects and sends metrics every 30 seconds
func (e *enhanceIndexingS3Exporter) startMetricsCollection(ctx context.Context) {
	e.logger.Info("Starting metrics collection for standalone mode")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.done:
			return
		case <-ticker.C:
			e.collectAndSendMetrics(ctx)
		}
	}
}

// collectAndSendMetrics collects metrics and sends them to the configured endpoint
// Metrics are sent to: POST {apiEndpoint}/2/teams/{teamSlug}/enhance_indexer_usage
// Request format (JSONAPI):
//
//	{
//	  "data": {
//	    "type": "enhance_indexer_usage",
//	    "id": "enhance_indexing_s3_exporter",
//	    "attributes": {
//	      "s3Bucket": "{bucket_name}",
//	      "s3FilePrefix": "{prefix}",
//	      "usageData": {
//	        // OpenTelemetry metrics in JSON format
//	      }
//	    }
//	  }
//	}
//
// Response: 200 OK or 204 No Content on success
func (e *enhanceIndexingS3Exporter) collectAndSendMetrics(ctx context.Context) {
	metrics := e.createUsageReport()

	if metrics.MetricCount() == 0 {
		e.logger.Debug("No metrics to send")
		return
	}

	if e.teamSlug == "" {
		e.logger.Debug("No team slug configured, skipping metrics send")
		return
	}

	requestData := enhanceIndexerUsageRecordRequest{
		Data: enhanceIndexerUsageRecordContents{
			Type: "enhance_indexer_usage",
			ID:   typeStr,
			Attributes: createEnhanceIndexerUsageRecordAttributes{
				S3Bucket:     e.config.S3Uploader.S3Bucket,
				S3FilePrefix: e.config.S3Uploader.FilePrefix,
				UsageData: &usageMetrics{
					Metrics: metrics,
				},
			},
		},
	}

	data, err := json.Marshal(requestData)
	if err != nil {
		e.logger.Error("Failed to marshal metrics request", zap.Error(err))
		return
	}

	url := fmt.Sprintf("%s/2/teams/%s/enhance_indexer_usage", e.config.APIEndpoint, e.teamSlug)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		e.logger.Error("Failed to create metrics request", zap.Error(err))
		return
	}

	auth := fmt.Sprintf("Bearer %s:%s", string(e.config.APIKey), string(e.config.APISecret))
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	resp, err := otelhttp.DefaultClient.Do(req)
	if err != nil {
		e.logger.Error("Failed to send metrics", zap.Error(err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent {
		e.logger.Debug("Successfully sent metrics", zap.Int("metric_count", metrics.MetricCount()))
		return
	}

	e.logger.Error("Failed to send metrics", zap.Int("status_code", resp.StatusCode))
}
