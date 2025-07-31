package enhanceindexings3exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type fieldName string
type fieldValue string
type fieldS3Keys []string

// MinuteIndexBatch holds index data for a one minute period
type MinuteIndexBatch struct {
	minuteDir    string                                   // "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=00"
	fieldIndexes map[fieldName]map[fieldValue]fieldS3Keys // field_name -> {field_value -> slice of s3 keys}
}

type enhanceIndexingS3Exporter struct {
	config             *Config
	logger             *zap.Logger
	s3Writer           *S3Writer
	timer              *time.Timer
	indexMutex         sync.RWMutex
	minuteIndexBatches map[int]*MinuteIndexBatch
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
		return fmt.Errorf("s3 bucket name is empty")
	}

	e.s3Writer = NewS3Writer(&e.config.S3Uploader, e.config.MarshalerName, s3Client, e.logger)

	// Start index checking and uploading timer if indexing is enabled
	if e.config.IndexConfig.Enabled {
		e.startTimer(ctx)
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) shutdown(ctx context.Context) error {
	// Stop the minute timer and upload any pending indexes
	if e.timer != nil {
		e.timer.Stop()
	}

	// Upload any remaining batch data
	e.indexMutex.Lock()
	if len(e.minuteIndexBatches) > 0 {
		e.logger.Info("Uploading remaining index data", zap.Int("batchCount", len(e.minuteIndexBatches)))
		for minute, batch := range e.minuteIndexBatches {
			err := e.uploadBatch(ctx, batch)
			if err != nil {
				e.logger.Error("Failed to upload remaining index data", zap.Error(err))
				e.indexMutex.Unlock()
				return err
			}

			e.logger.Info("Uploaded index batch for the minute", zap.Int("minute", minute))
			delete(e.minuteIndexBatches, minute)
		}
	}

	e.indexMutex.Unlock()

	return nil
}

// startTimer starts a timer that triggers every 30 seconds, which will check for
// index batches that are ready to be uploaded and uploads them. It also initializes
// an empty index batch for the current minute.
func (e *enhanceIndexingS3Exporter) startTimer(ctx context.Context) {
	minute := time.Now().UTC().Minute()
	e.logger.Info("Starting index batch timer")

	// Initialize an empty index batch for the current minute
	e.minuteIndexBatches = map[int]*MinuteIndexBatch{}
	e.minuteIndexBatches[minute] = &MinuteIndexBatch{
		fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
	}

	// Wait for 1 second before starting the timer
	e.timer = time.AfterFunc(time.Second, func() {
		// Set up a recurring timer for every 30 seconds
		ticker := time.NewTicker(30 * time.Second)
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					minute := time.Now().UTC().Minute()
					e.logger.Info("Timer ticked, checking for index batches to upload", zap.Int("minute", minute))

					// Check if there are any index batches that are ready to be uploaded
					for minute, indexBatch := range e.minuteIndexBatches {
						if e.readyToUpload(minute) {
							e.logger.Info("Index batch is ready to be uploaded", zap.Int("minute", minute))
							err := e.uploadBatch(ctx, indexBatch)
							if err != nil {
								e.logger.Error("Failed to upload index batch", zap.Error(err))
								break
							}

							e.logger.Info("Uploaded index batch for the minute", zap.Int("minute", minute))
							delete(e.minuteIndexBatches, minute)
						}
					}

					// Initialize an empty index batch for the current minute
					e.minuteIndexBatches = map[int]*MinuteIndexBatch{}
					e.minuteIndexBatches[minute] = &MinuteIndexBatch{
						fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
					}
				}
			}
		}()
	})
}

// readyToUpload checks if the minute batch is ready to be uploaded
// If the current minute is not equal to the minute of the index batch, the index batch is ready to be uploaded
func (e *enhanceIndexingS3Exporter) readyToUpload(minute int) bool {
	return time.Now().UTC().Minute() != minute
}

// addToIndex adds trace field information to the current minute's MinuteIndexBatch
// assuming that the field is configured to be indexed. Trace ID is always indexed.
// Additional fields are indexed if they are present in configuration.
// The minute comes from the s3Key generated by the s3Writer.WriteBuffer function
func (e *enhanceIndexingS3Exporter) addToIndex(traces ptrace.Traces, s3Key string, minute int) {
	e.indexMutex.Lock()
	defer e.indexMutex.Unlock()
	currentBatch := e.minuteIndexBatches[minute]

	currentBatch.minuteDir = filepath.Dir(s3Key)

	// Extract and add field values to current batch
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)

				// trace id is always indexed
				traceID := span.TraceID().String()
				traceIDFName := fieldName("trace_id")
				traceIDFVal := fieldValue(traceID)

				if _, ok := currentBatch.fieldIndexes[traceIDFName]; !ok {
					currentBatch.fieldIndexes[traceIDFName] = map[fieldValue]fieldS3Keys{}
				}

				// Append the S3 key to the trace id field index if it is not already present
				if !slices.Contains(currentBatch.fieldIndexes[traceIDFName][traceIDFVal], s3Key) {
					currentBatch.fieldIndexes[traceIDFName][traceIDFVal] = append(currentBatch.fieldIndexes[traceIDFName][traceIDFVal], s3Key)
				}

				// Index configured fields
				span.Attributes().Range(func(attrKey string, v pcommon.Value) bool {
					// Check if the field is configured to be indexed
					if slices.Contains(e.config.IndexConfig.IndexedFields, fieldName(attrKey)) {
						fn := fieldName(attrKey)
						fv := fieldValue(v.AsString())

						if _, ok := currentBatch.fieldIndexes[fn]; !ok {
							currentBatch.fieldIndexes[fn] = map[fieldValue]fieldS3Keys{}
						}

						// Append the S3 key to the field value index if it is not already present
						if !slices.Contains(currentBatch.fieldIndexes[fn][fv], s3Key) {
							currentBatch.fieldIndexes[fn][fv] = append(currentBatch.fieldIndexes[fn][fv], s3Key)
						}
					}

					return true
				})
			}
		}
	}
}

// marshalIndex marshals the index using the configured marshaler type
func (e *enhanceIndexingS3Exporter) marshalIndex(fIndex map[fieldValue]fieldS3Keys) ([]byte, error) {
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		return json.Marshal(fIndex)
	} else {
		// For protobuf, we need to manually encode since index isn't an OTel signal
		// We'll create a simple protobuf-compatible format
		return e.marshalIndexAsProtobuf(fIndex)
	}
}

// marshalIndexAsProtobuf manually encodes the index as protobuf
func (e *enhanceIndexingS3Exporter) marshalIndexAsProtobuf(fIndex map[fieldValue]fieldS3Keys) ([]byte, error) {
	// For now, we'll use a simple approach: encode as JSON first, then wrap in a protobuf-like structure
	// In a production environment, you would define a proper .proto file and generate Go structs
	jsonData, err := json.Marshal(fIndex)
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
func (e *enhanceIndexingS3Exporter) uploadBatch(ctx context.Context, batch *MinuteIndexBatch) error {
	e.logger.Info("Uploading index batch")

	for fName, fIndex := range batch.fieldIndexes {
		if len(fIndex) == 0 {
			e.logger.Info("No index data to upload", zap.String("field", string(fName)))
			continue
		}

		indexData, err := e.marshalIndex(fIndex)
		if err != nil {
			e.logger.Error("Failed to marshal index", zap.Error(err), zap.String("field", string(fName)))
			return err
		}

		// Determine file extension based on marshaler
		var fileExt string
		if e.config.MarshalerName == awss3exporter.OtlpJSON {
			fileExt = "json"
		} else {
			fileExt = "binpb" // binary protobuf
		}

		indexKey := fmt.Sprintf("%s/index_%s_%s.%s", batch.minuteDir, string(fName), uuid.New().String(), fileExt)
		if e.config.S3Uploader.Compression == "gzip" {
			indexKey += ".gz"
		}

		_, _, err = e.s3Writer.WriteBufferWithIndex(ctx, indexData, "index", indexKey)
		if err != nil {
			e.logger.Error("Failed to upload index", zap.Error(err), zap.String("field", string(fName)))
			return err
		}

		e.logger.Info("Uploaded index", zap.String("field", string(fName)), zap.String("key", indexKey), zap.String("format", string(e.config.MarshalerName)))
	}

	return nil
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
	s3Key, minute, err := e.s3Writer.WriteBuffer(ctx, buf, "traces")
	if err != nil {
		return err
	}

	// Add to index batch if enabled
	if e.config.IndexConfig.Enabled {
		if _, ok := e.minuteIndexBatches[minute]; !ok {
			e.logger.Info("No index batch found for current minute, creating empty index batch before adding to index", zap.Int("minute", minute))
			e.minuteIndexBatches[minute] = &MinuteIndexBatch{
				fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
			}
		}

		e.addToIndex(traces, s3Key, minute)
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
	_, _, err = e.s3Writer.WriteBuffer(ctx, buf, "logs")
	return err
}
