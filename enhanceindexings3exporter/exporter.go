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

	"github.com/honeycombio/enhance-indexing-s3-exporter/index"
)

type fieldName string
type fieldValue string
type fieldS3Keys []string

// MinuteIndexBatch holds index data for a one minute period
type MinuteIndexBatch struct {
	minuteDir    string                                   // "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=00"
	fieldIndexes map[fieldName]map[fieldValue]fieldS3Keys // field_name -> {field_value -> slice of s3 keys}
}

// IndexManager manages shared index state across multiple exporters
type IndexManager struct {
	mutex              sync.RWMutex
	minuteIndexBatches map[int]*MinuteIndexBatch
	ticker             *time.Ticker
	config             *Config
	logger             *zap.Logger
	s3Writer           S3WriterInterface
}

type enhanceIndexingS3Exporter struct {
	config       *Config
	logger       *zap.Logger
	s3Writer     S3WriterInterface
	indexManager *IndexManager
}

func newEnhanceIndexingS3Exporter(cfg *Config, logger *zap.Logger, indexManager *IndexManager) (*enhanceIndexingS3Exporter, error) {
	return &enhanceIndexingS3Exporter{
		config:       cfg,
		logger:       logger,
		indexManager: indexManager,
	}, nil
}

// NewIndexManager creates a new IndexManager
func NewIndexManager(config *Config, logger *zap.Logger) *IndexManager {
	if config.IndexConfig.Enabled {
		if !slices.Contains(config.IndexConfig.IndexedFields, fieldName("session.id")) {
			config.IndexConfig.IndexedFields = append(config.IndexConfig.IndexedFields, fieldName("session.id"))
		}
	}

	return &IndexManager{
		minuteIndexBatches: make(map[int]*MinuteIndexBatch),
		config:             config,
		logger:             logger,
	}
}

// ensureMinuteBatch ensures that the minute batch exists for the given minute
// by creating an empty MinuteIndexBatch with the given minute
// and adding it to the index manager's minuteIndexBatches map
func (im *IndexManager) ensureMinuteBatch(minute int) {
	im.mutex.Lock()
	defer im.mutex.Unlock()
	if _, ok := im.minuteIndexBatches[minute]; !ok {
		im.minuteIndexBatches[minute] = &MinuteIndexBatch{
			fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
		}
	}
}

// start initializes the IndexManager
func (im *IndexManager) start(ctx context.Context, s3Writer S3WriterInterface) error {
	im.s3Writer = s3Writer

	// Initialize an empty index batch for the current minute
	minute := time.Now().UTC().Minute()
	im.ensureMinuteBatch(minute)
	im.startTimer(ctx)
	return nil
}

// shutdown stops the IndexManager
func (im *IndexManager) shutdown(ctx context.Context) error {
	// Stop the minute ticker and upload any pending indexes. There might be an upload in progress.
	if im.ticker != nil {
		im.ticker.Stop()
	}

	// TODO figure out if we need to wait for the upload to finish before continuing

	// Upload any remaining batch data
	im.mutex.Lock()
	defer im.mutex.Unlock()

	if len(im.minuteIndexBatches) > 0 {
		im.logger.Info("Uploading remaining index data", zap.Int("batchCount", len(im.minuteIndexBatches)))
		for minute, batch := range im.minuteIndexBatches {
			err := im.uploadBatch(ctx, batch)
			if err != nil {
				im.logger.Error("Failed to upload remaining index data", zap.Error(err))
				return err
			}

			im.logger.Info("Uploaded index batch for the minute", zap.Int("minute", minute))
			delete(im.minuteIndexBatches, minute)
		}
	}

	return nil
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

	// Initialize IndexManager if indexing is enabled
	if e.config.IndexConfig.Enabled && e.indexManager != nil {
		var startOnce sync.Once

		// Ensure that the index manager is started only once
		startOnce.Do(func() {
			err := e.indexManager.start(ctx, e.s3Writer)
			if err != nil {
				e.logger.Error("Failed to start index manager", zap.Error(err))
			}
		})
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) shutdown(ctx context.Context) error {
	if e.config.IndexConfig.Enabled && e.indexManager != nil {
		var shutdownOnce sync.Once

		// Ensure that the index manager is shutdown only once
		shutdownOnce.Do(func() {
			err := e.indexManager.shutdown(ctx)
			if err != nil {
				e.logger.Error("Failed to shutdown index manager", zap.Error(err))
			}
		})
	}
	return nil
}

// startTimer starts a timer that triggers every 30 seconds, which will check for
// index batches that are ready to be uploaded and uploads them. It also initializes
// an empty index batch for the current minute.
func (im *IndexManager) startTimer(ctx context.Context) {
	im.logger.Info("Starting index batch timer")

	// Set up a recurring timer for every 30 seconds - the ticker is stopped in the shutdown function
	im.ticker = time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-im.ticker.C:
				// blocking so we don't have multiple rollovers running simultaneously
				im.rolloverIndexes(ctx)
			}
		}
	}()
}

func (im *IndexManager) rolloverIndexes(ctx context.Context) {
	minute := time.Now().UTC().Minute()
	im.logger.Info("Timer ticked, checking for index batches to upload", zap.Int("minute", minute))

	// Check if there are any index batches that are ready to be uploaded
	for minute, indexBatch := range im.minuteIndexBatches {
		if im.readyToUpload(minute) {
			im.mutex.RLock()
			im.logger.Info("Index batch is ready to be uploaded", zap.Int("minute", minute))
			err := im.uploadBatch(ctx, indexBatch)
			im.mutex.RUnlock()
			if err != nil {
				im.logger.Error("Failed to upload index batch", zap.Error(err))
				break
			}

			im.logger.Info("Deleting index batch for the minute", zap.Int("minute", minute))
			delete(im.minuteIndexBatches, minute)
		}
	}

	// Initialize an empty index batch for the current minute if it doesn't exist
	im.ensureMinuteBatch(minute)
}

// readyToUpload checks if the minute batch is ready to be uploaded
// If the current minute is not equal to the minute of the index batch, the index batch is ready to be uploaded
func (im *IndexManager) readyToUpload(minute int) bool {
	return time.Now().UTC().Minute() != minute
}

// addTracesToIndex adds trace field information to the current minute's
// MinuteIndexBatch assuming that the field is configured to be indexed.  Trace
// ID is always indexed and will be extracted from the span.TraceID().String()
// method.  Additional fields are indexed if they are present in configuration.
// The minute passed in comes from the s3Key generated by the
// s3Writer.WriteBuffer function
func (im *IndexManager) addTracesToIndex(traces ptrace.Traces, s3Key string, minute int) {
	// Ensure the batch exists for this minute
	im.ensureMinuteBatch(minute)

	im.mutex.Lock()
	defer im.mutex.Unlock()

	currentBatch := im.minuteIndexBatches[minute]
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
					if slices.Contains(im.config.IndexConfig.IndexedFields, fieldName(attrKey)) {
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
func (im *IndexManager) marshalIndex(fieldName string, fieldIndex map[fieldValue]fieldS3Keys) ([]byte, error) {
	if im.config.MarshalerName == awss3exporter.OtlpJSON {
		return json.Marshal(fieldIndex)
	} else {
		// For protobuf, we use the generated protobuf methods
		return im.marshalIndexAsProtobuf(fieldName, fieldIndex)
	}
}

// marshalIndexAsProtobuf encodes the index using generated protobuf methods
func (im *IndexManager) marshalIndexAsProtobuf(fieldName string, fieldIndex map[fieldValue]fieldS3Keys) ([]byte, error) {
	// Create the protobuf FieldIndex structure
	fieldIndexProto := &index.FieldIndex{
		FieldName:  fieldName,
		FieldIndex: make(map[string]*index.S3Keys),
	}

	// Convert the map data to protobuf structures
	for fieldVal, s3Keys := range fieldIndex {
		s3KeysList := &index.S3Keys{
			S3Keys: make([]string, len(s3Keys)),
		}
		copy(s3KeysList.S3Keys, s3Keys)
		fieldIndexProto.FieldIndex[string(fieldVal)] = s3KeysList
	}

	// Use the generated Marshal method
	return fieldIndexProto.Marshal()
}

// uploadBatch uploads all index files for a completed minute batch
func (im *IndexManager) uploadBatch(ctx context.Context, batch *MinuteIndexBatch) error {
	if len(batch.fieldIndexes) == 0 {
		im.logger.Info("No index data to upload")
		return nil
	}

	for fName, fIndex := range batch.fieldIndexes {
		indexData, err := im.marshalIndex(string(fName), fIndex)
		if err != nil {
			im.logger.Error("Failed to marshal index", zap.Error(err), zap.String("field", string(fName)))
			return err
		}

		// Determine file extension based on marshaler
		var fileExt string
		if im.config.MarshalerName == awss3exporter.OtlpJSON {
			fileExt = "json"
		} else {
			fileExt = "binpb" // binary protobuf
		}

		indexKey := fmt.Sprintf("%s/index_%s_%s.%s", batch.minuteDir, string(fName), uuid.New().String(), fileExt)
		if im.config.S3Uploader.Compression == "gzip" {
			indexKey += ".gz"
		}

		_, _, err = im.s3Writer.WriteBufferWithIndex(ctx, indexData, "index", indexKey)
		if err != nil {
			im.logger.Error("Failed to upload index", zap.Error(err), zap.String("field", string(fName)))
			return err
		}

		im.logger.Info("Uploaded index", zap.String("field", string(fName)), zap.String("key", indexKey), zap.String("format", string(im.config.MarshalerName)))
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
	if e.config.IndexConfig.Enabled && e.indexManager != nil {
		e.indexManager.addTracesToIndex(traces, s3Key, minute)
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
