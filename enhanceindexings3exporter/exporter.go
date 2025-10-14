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
	"go.opentelemetry.io/otel/attribute"
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
	startOnce          sync.Once
	shutdownOnce       sync.Once
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
	metrics      *ExporterMetrics
}

// These are the fields that are automatically indexed. Note that trace id is
// also automatically indexed but handled as a special case using different
// methods for traces and logs.
var automaticallyIndexedFields = []string{"service.name", "session.id"}

// buildIndexesFromAttributes looks through the Attributes of Resources, Scopes,
// and LogRecords/Spans and adds them to the current batch's field indexes if
// they are not already present. To ensure precedence is respected (Item > Scope
// > Resource), the lower precedence field value is returned as Attributes are
// evaluated in reverse precedence order, and that value is passed in when
// evaluating the next Attribute type.
func buildIndexesFromAttributes(
	currentBatch *MinuteIndexBatch,
	attrs pcommon.Map,
	indexedFields []fieldName,
	s3Key string,
	previousFV fieldValue,
) fieldValue {
	var fv fieldValue

	for _, field := range indexedFields {
		attrFieldValue, ok := attrs.Get(string(field))
		if !ok {
			continue
		}

		fn := fieldName(field)
		fv = fieldValue(attrFieldValue.AsString())
		if _, ok := currentBatch.fieldIndexes[fn]; !ok {
			currentBatch.fieldIndexes[fn] = map[fieldValue]fieldS3Keys{}
		}

		// Remove the S3 key from the previous field index's value if it is present
		if _, ok := currentBatch.fieldIndexes[fn][previousFV]; ok && previousFV != "" {
			currentBatch.fieldIndexes[fn][previousFV] = slices.DeleteFunc(currentBatch.fieldIndexes[fn][previousFV], func(s string) bool {
				return s == s3Key
			})

			if len(currentBatch.fieldIndexes[fn][previousFV]) == 0 {
				delete(currentBatch.fieldIndexes[fn], previousFV)
			}
		}

		// Append the S3 key to the field value index if it is not already present
		if !slices.Contains(currentBatch.fieldIndexes[fn][fv], s3Key) {
			currentBatch.fieldIndexes[fn][fv] = append(currentBatch.fieldIndexes[fn][fv], s3Key)
		}
	}

	return fv
}

func newEnhanceIndexingS3Exporter(cfg *Config, logger *zap.Logger, indexManager *IndexManager) (*enhanceIndexingS3Exporter, error) {
	// Create metrics with OpenTelemetry instrumentation
	metrics, err := NewExporterMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter metrics: %w", err)
	}

	return &enhanceIndexingS3Exporter{
		config:       cfg,
		logger:       logger,
		indexManager: indexManager,
		metrics:      metrics,
	}, nil
}

// NewIndexManager creates a new IndexManager
func NewIndexManager(config *Config, logger *zap.Logger) *IndexManager {
	// Add all automatically indexed fields to the index config's indexed fields list if they are not already present
	for _, field := range automaticallyIndexedFields {
		if !slices.Contains(config.IndexedFields, fieldName(field)) {
			config.IndexedFields = append(config.IndexedFields, fieldName(field))
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
	im.startOnce.Do(func() {
		im.s3Writer = s3Writer

		// Initialize an empty index batch for the current minute
		minute := time.Now().UTC().Minute()
		im.ensureMinuteBatch(minute)
		im.startTimer(ctx)
	})
	return nil
}

// shutdown stops the IndexManager
func (im *IndexManager) shutdown(ctx context.Context) error {
	im.shutdownOnce.Do(func() {
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
					break
				}

				im.logger.Info("Uploaded index batch for the minute", zap.Int("minute", minute))
				delete(im.minuteIndexBatches, minute)
			}
		}
	})
	return nil
}

func (e *enhanceIndexingS3Exporter) start(ctx context.Context, host component.Host) error {
	e.logger.Info("Starting enhance indexing S3 exporter",
		zap.String("region", e.config.S3Uploader.Region),
		zap.String("api_endpoint", e.config.APIEndpoint))

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

	if e.indexManager != nil {
		err := e.indexManager.start(ctx, e.s3Writer)
		if err != nil {
			e.logger.Error("Failed to start index manager", zap.Error(err))
			return err
		}
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) shutdown(ctx context.Context) error {
	if e.indexManager != nil {
		err := e.indexManager.shutdown(ctx)
		if err != nil {
			e.logger.Error("Failed to shutdown index manager", zap.Error(err))
			return err
		}
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
// MinuteIndexBatch assuming that the field is configured to be indexed. Trace
// ID is always indexed and will be extracted from the span.TraceID().String()
// method. session.id and service.name are also always indexed. Additional
// custom fields are indexed if they are present in configuration. The minute
// passed in comes from the s3Key generated by the s3Writer.WriteBuffer
// function.
func (im *IndexManager) addTracesToIndex(traces ptrace.Traces, s3Key string, minute int) {
	// Ensure the batch exists for this minute
	im.ensureMinuteBatch(minute)

	im.mutex.Lock()
	defer im.mutex.Unlock()

	currentBatch := im.minuteIndexBatches[minute]
	currentBatch.minuteDir = filepath.Dir(s3Key)

	// Extract and add field values to current batch
	// The order of precedence is (with 1 being highest):
	// 1. Item (span) attributes
	// 2. Instrumentation scope attributes
	// 3. Resource attributes
	var previousFV fieldValue = fieldValue("")

	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		// Extract Resource attributes first
		previousFV = buildIndexesFromAttributes(currentBatch, rs.Resource().Attributes(), im.config.IndexedFields, s3Key, previousFV)

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			// Extract Instrumentation scope attributes next
			previousFV = buildIndexesFromAttributes(currentBatch, ss.Scope().Attributes(), im.config.IndexedFields, s3Key, previousFV)

			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				// Extract span attributes last, at highest precedence
				_ = buildIndexesFromAttributes(currentBatch, span.Attributes(), im.config.IndexedFields, s3Key, previousFV)

				// trace id is always indexed from ptrace.Span
				traceID := span.TraceID().String()
				traceIDFName := fieldName("trace.trace_id")
				traceIDFVal := fieldValue(traceID)

				if _, ok := currentBatch.fieldIndexes[traceIDFName]; !ok {
					currentBatch.fieldIndexes[traceIDFName] = map[fieldValue]fieldS3Keys{}
				}

				// Append the S3 key to the trace id field index if it is not already present
				if !slices.Contains(currentBatch.fieldIndexes[traceIDFName][traceIDFVal], s3Key) {
					currentBatch.fieldIndexes[traceIDFName][traceIDFVal] = append(currentBatch.fieldIndexes[traceIDFName][traceIDFVal], s3Key)
				}
			}
		}
	}
}

// addLogsToIndex adds log field information to the current minute's
// MinuteIndexBatch assuming that the field is configured to be indexed. Trace
// ID is always indexed and will be extracted from the log.TraceID().String()
// method. session.id and service.name are also always indexed. Additional
// custom fields are indexed if they are present in configuration. The minute
// passed in comes from the s3Key generated by the s3Writer.WriteBuffer
// function.
func (im *IndexManager) addLogsToIndex(logs plog.Logs, s3Key string, minute int) {
	// Ensure the batch exists for this minute
	im.ensureMinuteBatch(minute)

	im.mutex.Lock()
	defer im.mutex.Unlock()

	currentBatch := im.minuteIndexBatches[minute]
	currentBatch.minuteDir = filepath.Dir(s3Key)

	// Extract and add field values to current batch
	// The order of precedence is (with 1 being highest):
	// 1. Item (log record) attributes
	// 2. Instrumentation scope attributes
	// 3. Resource attributes
	var previousFV fieldValue = fieldValue("")

	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		// Extract Resource attributes first
		previousFV = buildIndexesFromAttributes(currentBatch, rl.Resource().Attributes(), im.config.IndexedFields, s3Key, previousFV)

		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			// Extract Instrumentation scope attributes next
			previousFV = buildIndexesFromAttributes(currentBatch, sl.Scope().Attributes(), im.config.IndexedFields, s3Key, previousFV)

			for k := 0; k < sl.LogRecords().Len(); k++ {
				log := sl.LogRecords().At(k)
				// Extract log record attributes last, at highest precedence
				_ = buildIndexesFromAttributes(currentBatch, log.Attributes(), im.config.IndexedFields, s3Key, previousFV)

				// trace id is always indexed from plog.LogRecord
				traceID := log.TraceID().String()

				// Trace ID in a plog.LogRecord is specifically defined as an
				// optional field, so we only index it if it is present
				if traceID != "" {
					traceIDFName := fieldName("trace.trace_id")
					traceIDFVal := fieldValue(traceID)

					if _, ok := currentBatch.fieldIndexes[traceIDFName]; !ok {
						currentBatch.fieldIndexes[traceIDFName] = map[fieldValue]fieldS3Keys{}
					}

					// Append the S3 key to the trace id field index if it is not already present
					if !slices.Contains(currentBatch.fieldIndexes[traceIDFName][traceIDFVal], s3Key) {
						currentBatch.fieldIndexes[traceIDFName][traceIDFVal] = append(currentBatch.fieldIndexes[traceIDFName][traceIDFVal], s3Key)
					}
				}
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

		// Log usage information including hostname for usage endpoint tracking
		logFields := []zap.Field{
			zap.String("field", string(fName)),
			zap.String("key", indexKey),
			zap.String("format", string(im.config.MarshalerName)),
		}
		if im.config.APIEndpoint != "" {
			logFields = append(logFields, zap.String("api_endpoint", im.config.APIEndpoint))
		}
		im.logger.Info("Uploaded index", logFields...)
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) consumeTraces(ctx context.Context, traces ptrace.Traces) error {
	spanCount := int64(traces.SpanCount())
	logFields := []zap.Field{zap.Int64("spanCount", spanCount)}
	if e.config.APIEndpoint != "" {
		logFields = append(logFields, zap.String("api_endpoint", e.config.APIEndpoint))
	}
	e.logger.Info("Consuming traces", logFields...)

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

	spanBytes := int64(len(buf))
	e.logger.Info("Uploading traces",
		zap.Int64("traceSpanCount", spanCount),
		zap.Int64("traceSpanBytes", spanBytes))

	s3Key, minute, err := e.s3Writer.WriteBuffer(ctx, buf, "traces")
	if err != nil {
		return err
	}

	// Record metrics with OpenTelemetry instrumentation
	attrs := []attribute.KeyValue{
		attribute.String("marshaler", string(e.config.MarshalerName)),
	}
	if e.config.APIEndpoint != "" {
		attrs = append(attrs, attribute.String("api_endpoint", e.config.APIEndpoint))
	}
	e.metrics.AddSpanMetrics(ctx, spanCount, spanBytes, attrs)

	// Add to index batch if enabled
	if e.indexManager != nil {
		e.indexManager.addTracesToIndex(traces, s3Key, minute)
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) consumeLogs(ctx context.Context, logs plog.Logs) error {
	logCount := int64(logs.LogRecordCount())
	logFields := []zap.Field{zap.Int64("logRecordCount", logCount)}
	if e.config.APIEndpoint != "" {
		logFields = append(logFields, zap.String("api_endpoint", e.config.APIEndpoint))
	}
	e.logger.Info("Consuming logs", logFields...)

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

	logBytes := int64(len(buf))
	e.logger.Info("Uploading logs",
		zap.Int64("logRecordCount", logCount),
		zap.Int64("logRecordBytes", logBytes))

	s3Key, minute, err := e.s3Writer.WriteBuffer(ctx, buf, "logs")
	if err != nil {
		return err
	}

	// Record metrics with OpenTelemetry instrumentation
	attrs := []attribute.KeyValue{
		attribute.String("marshaler", string(e.config.MarshalerName)),
	}
	if e.config.APIEndpoint != "" {
		attrs = append(attrs, attribute.String("api_endpoint", e.config.APIEndpoint))
	}
	e.metrics.AddLogMetrics(ctx, logCount, logBytes, attrs)

	// Add to index batch if enabled
	if e.indexManager != nil {
		e.indexManager.addLogsToIndex(logs, s3Key, minute)
	}

	return nil
}
