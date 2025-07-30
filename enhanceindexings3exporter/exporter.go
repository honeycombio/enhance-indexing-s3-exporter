package enhanceindexings3exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type IndexField struct {
	FieldValue string `json:"field_value"`
	S3Location string `json:"s3_location"`
}

// MinuteIndexBatch holds index data for one minute period
type MinuteIndexBatch struct {
	minute       string                       // "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=00"
	fieldIndexes map[string]map[string]string // field_name -> {field_value -> s3_key}
}

type enhanceIndexingS3Exporter struct {
	config        *Config
	logger        *zap.Logger
	s3Writer      *S3Writer
	currentMinute string
	currentBatch  *MinuteIndexBatch
	minuteTimer   *time.Timer
	indexMutex    sync.RWMutex
}

func newEnhanceIndexingS3Exporter(cfg *Config, logger *zap.Logger) (*enhanceIndexingS3Exporter, error) {
	return &enhanceIndexingS3Exporter{
		config: cfg,
		logger: logger,
	}, nil
}

func (e *enhanceIndexingS3Exporter) start(ctx context.Context, host component.Host) error {
	e.logger.Info("Starting enhance indexing S3 exporter", zap.String("region", e.config.S3Uploader.Region))

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(e.config.S3Uploader.Region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsConfig, func(o *s3.Options) {
		if e.config.S3Uploader.Endpoint != "" {
			o.BaseEndpoint = aws.String(e.config.S3Uploader.Endpoint)
		}
		o.UsePathStyle = e.config.S3Uploader.S3ForcePathStyle
		if e.config.S3Uploader.DisableSSL {
			o.EndpointOptions.DisableHTTPS = true
		}
	})

	bucket := e.config.S3Uploader.S3Bucket
	if bucket == "" {
		return fmt.Errorf("S3 bucket name is empty")
	}

	e.s3Writer = NewS3Writer(&e.config.S3Uploader, e.config.MarshalerName, s3Client, e.logger)

	// Start minute boundary timer if indexing is enabled
	if e.config.IndexConfig.Enabled {
		e.startMinuteBoundaryTimer(ctx)
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) shutdown(ctx context.Context) error {
	// Stop the minute timer and upload any pending indexes
	if e.minuteTimer != nil {
		e.minuteTimer.Stop()
	}

	// Upload any remaining batch data
	e.indexMutex.Lock()
	if e.currentBatch != nil {
		batch := e.currentBatch
		e.currentBatch = nil
		e.indexMutex.Unlock()
		e.uploadBatch(context.Background(), batch)
	} else {
		e.indexMutex.Unlock()
	}

	return nil
}

// startMinuteBoundaryTimer starts a timer that triggers at minute boundaries
func (e *enhanceIndexingS3Exporter) startMinuteBoundaryTimer(ctx context.Context) {
	now := time.Now()
	nextMinute := now.Truncate(time.Minute).Add(time.Minute)
	initialDelay := nextMinute.Sub(now)

	e.logger.Info("Starting minute boundary timer",
		zap.Duration("initialDelay", initialDelay),
		zap.Time("nextMinuteBoundary", nextMinute))

	e.minuteTimer = time.AfterFunc(initialDelay, func() {
		e.onMinuteBoundary(ctx)

		// Set up recurring timer for every minute
		ticker := time.NewTicker(time.Minute)
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					e.onMinuteBoundary(ctx)
				}
			}
		}()
	})
}

// onMinuteBoundary is called when the clock crosses to a new minute
func (e *enhanceIndexingS3Exporter) onMinuteBoundary(ctx context.Context) {
	e.logger.Info("Minute boundary reached, uploading indexes")

	e.indexMutex.Lock()
	if e.currentBatch != nil {
		batch := e.currentBatch
		e.currentBatch = nil
		e.currentMinute = ""
		e.indexMutex.Unlock()

		// Upload the completed minute's indexes
		go e.uploadBatch(ctx, batch)
	} else {
		e.indexMutex.Unlock()
	}
}

// addToCurrentIndex adds trace data to the current minute's index batch
func (e *enhanceIndexingS3Exporter) addToCurrentIndex(traces ptrace.Traces, s3Key string) {
	e.indexMutex.Lock()
	defer e.indexMutex.Unlock()

	// Extract minute path from s3Key
	minute := filepath.Dir(s3Key)

	// Check if we need to start a new minute batch
	if e.currentMinute != minute {
		// Upload previous minute's indexes if they exist
		if e.currentBatch != nil {
			go e.uploadBatch(context.Background(), e.currentBatch)
		}

		// Start new minute batch
		e.currentMinute = minute
		e.currentBatch = &MinuteIndexBatch{
			minute:       minute,
			fieldIndexes: make(map[string]map[string]string),
		}

		// Initialize maps for each indexed field
		for _, fieldName := range e.config.IndexConfig.IndexedFields {
			e.currentBatch.fieldIndexes[fieldName] = make(map[string]string)
		}
		e.currentBatch.fieldIndexes["trace_id"] = make(map[string]string) // Always index trace_id
	}

	// Extract and add field values to current batch
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)

				// Always index trace ID
				e.currentBatch.fieldIndexes["trace_id"][span.TraceID().String()] = s3Key

				// Index configured fields
				span.Attributes().Range(func(attrKey string, v pcommon.Value) bool {
					if fieldMap, exists := e.currentBatch.fieldIndexes[attrKey]; exists {
						fieldMap[v.AsString()] = s3Key
					}
					return true
				})
			}
		}
	}
}

// marshalIndex marshals the index using the configured marshaler type
func (e *enhanceIndexingS3Exporter) marshalIndex(fieldIndex map[string]string) ([]byte, error) {
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		return json.Marshal(fieldIndex)
	} else {
		// For protobuf, we need to manually encode since index isn't an OTel signal
		// We'll create a simple protobuf-compatible format
		return e.marshalIndexAsProtobuf(fieldIndex)
	}
}

// marshalIndexAsProtobuf manually encodes the index as protobuf
func (e *enhanceIndexingS3Exporter) marshalIndexAsProtobuf(fieldIndex map[string]string) ([]byte, error) {
	// For now, we'll use a simple approach: encode as JSON first, then wrap in a protobuf-like structure
	// In a production environment, you would define a proper .proto file and generate Go structs
	jsonData, err := json.Marshal(fieldIndex)
	if err != nil {
		return nil, err
	}

	// Simple protobuf-like encoding: length-prefixed JSON data
	// This is a simplified approach - in production you'd use proper protobuf definitions
	result := make([]byte, 4+len(jsonData))
	// Write length as 4-byte big-endian integer
	result[0] = byte(len(jsonData) >> 24)
	result[1] = byte(len(jsonData) >> 16)
	result[2] = byte(len(jsonData) >> 8)
	result[3] = byte(len(jsonData))
	// Write JSON data
	copy(result[4:], jsonData)

	return result, nil
}

// uploadBatch uploads all index files for a completed minute batch
func (e *enhanceIndexingS3Exporter) uploadBatch(ctx context.Context, batch *MinuteIndexBatch) {
	for fieldName, fieldIndex := range batch.fieldIndexes {
		if len(fieldIndex) == 0 {
			continue
		}

		indexData, err := e.marshalIndex(fieldIndex)
		if err != nil {
			e.logger.Error("failed to marshal index", zap.Error(err), zap.String("field", fieldName))
			continue
		}

		// Determine file extension based on marshaler
		var fileExt string
		if e.config.MarshalerName == awss3exporter.OtlpJSON {
			fileExt = "json"
		} else {
			fileExt = "binpb" // binary protobuf
		}

		indexKey := fmt.Sprintf("%s/index-%s.%s", batch.minute, fieldName, fileExt)
		if e.config.S3Uploader.Compression == "gzip" {
			indexKey += ".gz"
		}

		_, err = e.s3Writer.WriteBufferWithIndex(ctx, indexData, "index", indexKey)
		if err != nil {
			e.logger.Error("failed to upload index", zap.Error(err), zap.String("field", fieldName))
		} else {
			e.logger.Info("uploaded index", zap.String("field", fieldName), zap.String("key", indexKey), zap.String("format", string(e.config.MarshalerName)))
		}
	}
}

func (e *enhanceIndexingS3Exporter) consumeTraces(ctx context.Context, traces ptrace.Traces) error {
	e.logger.Info("Consuming traces", zap.Int("spanCount", traces.SpanCount()))

	var marshaler ptrace.Marshaler
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		marshaler = &ptrace.JSONMarshaler{}
	} else {
		marshaler = &ptrace.ProtoMarshaler{}
	}

	buf, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return fmt.Errorf("failed to marshal traces: %w", err)
	}

	e.logger.Info("Uploading traces", zap.Int("traceSpanCount", traces.SpanCount()))
	s3Key, err := e.s3Writer.WriteBuffer(ctx, buf, "traces")
	if err != nil {
		return err
	}

	// Add to current index batch if enabled
	if e.config.IndexConfig.Enabled {
		e.addToCurrentIndex(traces, s3Key)
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) consumeLogs(ctx context.Context, logs plog.Logs) error {
	e.logger.Info("Consuming logs", zap.Int("logRecordCount", logs.LogRecordCount()))

	// TODO: Add log fields to index

	var marshaler plog.Marshaler
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		marshaler = &plog.JSONMarshaler{}
	} else {
		marshaler = &plog.ProtoMarshaler{}
	}

	buf, err := marshaler.MarshalLogs(logs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	e.logger.Info("Uploading logs", zap.Int("logRecordCount", logs.LogRecordCount()))
	_, err = e.s3Writer.WriteBuffer(ctx, buf, "logs")
	return err
}
