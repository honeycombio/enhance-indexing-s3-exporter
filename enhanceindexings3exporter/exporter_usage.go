package enhanceindexings3exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
)

const (
	// instrumentationScopeName is the distinct scope name for indexer metrics
	instrumentationScopeName = "github.com/honeycombio/enhance-indexing-s3-exporter"
)

var (
	pmetricJSONUnmarshaller = pmetric.JSONUnmarshaler{}
	pmetricJSONMarshaller   = pmetric.JSONMarshaler{}
)

// usageData tracks bytes and count for a signal type
type usageData struct {
	bytes int64
	count int64
}

// UsageData wraps pmetric.Metrics and implements custom JSON marshaling
type UsageData struct {
	Metrics pmetric.Metrics
}

func (u *UsageData) MarshalJSON() ([]byte, error) {
	return pmetricJSONMarshaller.MarshalMetrics(u.Metrics)
}

func (u *UsageData) UnmarshalJSON(data []byte) error {
	metrics, err := pmetricJSONUnmarshaller.UnmarshalMetrics(data)
	if err != nil {
		return err
	}
	u.Metrics = metrics
	return nil
}

// CreateEnhanceIndexerUsageRecordAttributes represents the attributes for the usage record
type CreateEnhanceIndexerUsageRecordAttributes struct {
	S3Bucket     string     `json:"s3Bucket"`
	S3FilePrefix string     `json:"s3FilePrefix"`
	UsageData    *UsageData `json:"usageData"`
}

// enhanceIndexerUsageRecordContents represents the data content for the request
type enhanceIndexerUsageRecordContents struct {
	Type       string                                    `json:"type"`
	ID         string                                    `json:"id"`
	Attributes CreateEnhanceIndexerUsageRecordAttributes `json:"attributes"`
}

// enhanceIndexerUsageRecordRequest represents the full JSONAPI request
type enhanceIndexerUsageRecordRequest struct {
	Data enhanceIndexerUsageRecordContents `json:"data"`
}

// RecordTracesUsage records traces usage for metrics reporting
func (e *enhanceIndexingS3Exporter) RecordTracesUsage(td ptrace.Traces) {
	var size int
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		marshaler := e.traceMarshaler.(*ptrace.JSONMarshaler)
		buf, err := marshaler.MarshalTraces(td)
		if err != nil {
			e.logger.Error("Failed to marshal traces for size calculation", zap.Error(err))
			return
		}
		size = len(buf)
	} else {
		marshaler := e.traceMarshaler.(*ptrace.ProtoMarshaler)
		size = marshaler.TracesSize(td)
	}

	if size == 0 {
		return
	}

	e.usageMutex.Lock()
	e.usageTraces.bytes += int64(size)
	e.usageTraces.count += int64(td.SpanCount())
	e.usageMutex.Unlock()
}

// RecordLogsUsage records logs usage for metrics reporting
func (e *enhanceIndexingS3Exporter) RecordLogsUsage(ld plog.Logs) {
	var size int
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		marshaler := e.logMarshaler.(*plog.JSONMarshaler)
		buf, err := marshaler.MarshalLogs(ld)
		if err != nil {
			e.logger.Error("Failed to marshal logs for size calculation", zap.Error(err))
			return
		}
		size = len(buf)
	} else {
		marshaler := e.logMarshaler.(*plog.ProtoMarshaler)
		size = marshaler.LogsSize(ld)
	}

	if size == 0 {
		return
	}

	e.usageMutex.Lock()
	e.usageLogs.bytes += int64(size)
	e.usageLogs.count += int64(ld.LogRecordCount())
	e.usageMutex.Unlock()
}

// createUsageReport creates a pmetric.Metrics report from the current usage data
// and resets the usage counters
func (e *enhanceIndexingS3Exporter) createUsageReport() pmetric.Metrics {
	// Copy current usage and reset
	e.usageMutex.Lock()
	tracesBytes := e.usageTraces.bytes
	tracesCount := e.usageTraces.count
	logsBytes := e.usageLogs.bytes
	logsCount := e.usageLogs.count
	e.usageTraces = usageData{}
	e.usageLogs = usageData{}
	e.usageMutex.Unlock()

	// Build pmetric.Metrics
	m := pmetric.NewMetrics()
	rm := m.ResourceMetrics().AppendEmpty()
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName(instrumentationScopeName)

	// bytes_received metric
	bytesMetric := sm.Metrics().AppendEmpty()
	bytesMetric.SetName("bytes_received")
	bytesSum := bytesMetric.SetEmptySum()
	bytesSum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	// Add traces datapoint
	if tracesBytes > 0 {
		dp := bytesSum.DataPoints().AppendEmpty()
		dp.SetIntValue(tracesBytes)
		dp.Attributes().PutStr("signal", "traces")
		dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	}

	// Add logs datapoint
	if logsBytes > 0 {
		dp := bytesSum.DataPoints().AppendEmpty()
		dp.SetIntValue(logsBytes)
		dp.Attributes().PutStr("signal", "logs")
		dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	}

	// count_received metric
	countMetric := sm.Metrics().AppendEmpty()
	countMetric.SetName("count_received")
	countSum := countMetric.SetEmptySum()
	countSum.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)

	// Add traces count datapoint
	if tracesCount > 0 {
		dp := countSum.DataPoints().AppendEmpty()
		dp.SetIntValue(tracesCount)
		dp.Attributes().PutStr("signal", "traces")
		dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	}

	// Add logs count datapoint
	if logsCount > 0 {
		dp := countSum.DataPoints().AppendEmpty()
		dp.SetIntValue(logsCount)
		dp.Attributes().PutStr("signal", "logs")
		dp.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	}

	return m
}

// startMetricsCollection starts a goroutine that collects and sends metrics every 30 seconds
func (e *enhanceIndexingS3Exporter) startMetricsCollection(ctx context.Context) {
	e.logger.Info("Starting metrics collection for standalone mode")

	e.metricsTicker = time.NewTicker(30 * time.Second)
	e.metricsStopChan = make(chan struct{})

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.metricsStopChan:
				return
			case <-e.metricsTicker.C:
				e.collectAndSendMetrics(ctx)
			}
		}
	}()
}

// stopMetricsCollection stops the metrics collection ticker
func (e *enhanceIndexingS3Exporter) stopMetricsCollection(ctx context.Context) {
	e.metricsShutdownOnce.Do(func() {
		if e.metricsTicker != nil {
			e.logger.Info("Stopping metrics collection")

			// Stop the ticker
			e.metricsTicker.Stop()

			// Signal the goroutine to stop
			close(e.metricsStopChan)

			// Send final metrics before shutdown
			e.collectAndSendMetrics(ctx)
		}
	})
}

// collectAndSendMetrics collects metrics and sends them to the configured endpoint
// Metrics are sent to: POST {apiEndpoint}/2/teams/{teamSlug}/enhance_indexer_usage
// Request format (JSONAPI):
//
//	{
//	  "data": {
//	    "type": "enhance_indexer_usage",
//	    "id": "{exporter_id}",
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

	// Construct the JSONAPI request
	requestData := enhanceIndexerUsageRecordRequest{
		Data: enhanceIndexerUsageRecordContents{
			Type: "enhance_indexer_usage",
			ID:   "", // ID is optional for this endpoint
			Attributes: CreateEnhanceIndexerUsageRecordAttributes{
				S3Bucket:     e.config.S3Uploader.S3Bucket,
				S3FilePrefix: e.config.S3Uploader.FilePrefix,
				UsageData: &UsageData{
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

	url := fmt.Sprintf("%s/2/teams/%s/enhance_indexer_usage", e.config.APIEndpoint, e.config.TeamSlug)

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
