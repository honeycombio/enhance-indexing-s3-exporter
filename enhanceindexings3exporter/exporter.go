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
type fieldS3Keys map[string]struct{}

// MinuteIndexBatch holds index data for a one minute period
type MinuteIndexBatch struct {
	// minuteDir is the directory for the minute batch
	// e.g. "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=00"
	minuteDir string
	//fieldIndexes looks like:
	// {field_name: {field_value: {s3_key: struct{}}}}
	// e.g. {trace.trace_id: {1234567890: {traces-and-logs/year=2025/month=07/day=28/hour=12/minute=00/traces_1234567890.binpb.gz: struct{}}}}
	fieldIndexes map[fieldName]map[fieldValue]fieldS3Keys
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
	config              *Config
	logger              *zap.Logger
	s3Writer            S3WriterInterface
	indexManager        *IndexManager
	traceMarshaler      ptrace.Marshaler
	logMarshaler        plog.Marshaler
	traceUsageMarshaler *ptrace.ProtoMarshaler
	logUsageMarshaler   *plog.ProtoMarshaler
	standaloneMode      bool
	teamSlug            string
	done                chan struct{}
	usageTraces         usageData
	usageTracesMutex    sync.Mutex
	usageLogs           usageData
	usageLogsMutex      sync.Mutex
}

// These are the fields that are automatically indexed. Note that trace id is
// also automatically indexed but handled as a special case using different
// methods for traces and logs.
var automaticallyIndexedFields = []string{"service.name", "session.id"}

// buildIndexesFromAttributes looks through the Attributes of Resources, Scopes,
// and LogRecords/Spans and records, for every indexed field value present, that
// this S3 key contains that value.
//
// The index is an inclusive file-level inverted index: its job is to answer
// "which S3 files contain value V for field F". A file must therefore be listed
// under every value it contains, at any attribute level. We deliberately do NOT
// apply OTel attribute precedence (Item > Scope > Resource) here: precedence
// resolves a single record's effective value, but a single S3 file batches many
// records (and, for bulk ingest, many tenants), so a value that appears only at
// resource scope is still genuinely present in the file. Deleting it would make
// the index under-report and cause index-based rehydrate to return zero results
// for that value even though the data is in the file (COR-3947).
func buildIndexesFromAttributes(
	currentBatch *MinuteIndexBatch,
	attrs pcommon.Map,
	indexedFields []fieldName,
	s3Key string,
) {
	for _, field := range indexedFields {
		attrFieldValue, ok := attrs.Get(string(field))
		if !ok {
			continue
		}

		fn := fieldName(field)
		fv := fieldValue(attrFieldValue.AsString())
		if _, ok := currentBatch.fieldIndexes[fn]; !ok {
			currentBatch.fieldIndexes[fn] = map[fieldValue]fieldS3Keys{}
		}

		// Add the S3 key to the field value index set. Never remove: the file
		// contains this value regardless of what other levels or records hold.
		if currentBatch.fieldIndexes[fn][fv] == nil {
			currentBatch.fieldIndexes[fn][fv] = make(fieldS3Keys)
		}
		currentBatch.fieldIndexes[fn][fv][s3Key] = struct{}{}
	}
}

// setMissingSpanTimestamps backfills the given timestamp onto any span whose
// start or end timestamp is unset (epoch 0). The Bulk Ingest service that
// Enhance uses does not stamp an arrival time on records that arrive without
// one, so they would otherwise be stored at the Unix epoch (1970-01-01). Both
// start and end are checked independently because such records arrive with both
// zeroed; guarding end avoids producing a negative duration when only start was
// backfilled.
//
// Span events carry their own timestamp and are stored as separate events, so
// they need the same treatment. They are backfilled from the span's start
// rather than from now, mirroring how span links inherit their parent span's
// timestamp, so that an event stays within its span and does not report a
// negative time since span start.
func setMissingSpanTimestamps(traces ptrace.Traces, now pcommon.Timestamp) {
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				if span.StartTimestamp() == 0 {
					span.SetStartTimestamp(now)
				}
				if span.EndTimestamp() == 0 {
					span.SetEndTimestamp(now)
				}

				events := span.Events()
				for e := 0; e < events.Len(); e++ {
					event := events.At(e)
					if event.Timestamp() == 0 {
						event.SetTimestamp(span.StartTimestamp())
					}
				}
			}
		}
	}
}

// setMissingLogTimestamps backfills the given timestamp onto any log record
// whose timestamp is unset (epoch 0), for the same reason as
// setMissingSpanTimestamps.
func setMissingLogTimestamps(logs plog.Logs, now pcommon.Timestamp) {
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				lr := sl.LogRecords().At(k)
				if lr.Timestamp() == 0 {
					lr.SetTimestamp(now)
				}
			}
		}
	}
}

func newEnhanceIndexingS3Exporter(cfg *Config, logger *zap.Logger, indexManager *IndexManager) (*enhanceIndexingS3Exporter, error) {
	var traceMarshaler ptrace.Marshaler
	var logMarshaler plog.Marshaler
	if cfg.MarshalerName == awss3exporter.OtlpJSON {
		traceMarshaler = &ptrace.JSONMarshaler{}
		logMarshaler = &plog.JSONMarshaler{}
	} else {
		traceMarshaler = &ptrace.ProtoMarshaler{}
		logMarshaler = &plog.ProtoMarshaler{}
	}

	return &enhanceIndexingS3Exporter{
		config:              cfg,
		logger:              logger,
		indexManager:        indexManager,
		traceMarshaler:      traceMarshaler,
		logMarshaler:        logMarshaler,
		traceUsageMarshaler: &ptrace.ProtoMarshaler{},
		logUsageMarshaler:   &plog.ProtoMarshaler{},
		done:                make(chan struct{}),
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
	im.ensureMinuteBatchLocked(minute)
}

// ensureMinuteBatchLocked is ensureMinuteBatch for callers that already hold
// im.mutex. It returns the batch for the minute, creating it if necessary.
func (im *IndexManager) ensureMinuteBatchLocked(minute int) *MinuteIndexBatch {
	batch, ok := im.minuteIndexBatches[minute]
	if !ok {
		batch = &MinuteIndexBatch{
			fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
		}
		im.minuteIndexBatches[minute] = batch
	}
	return batch
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
					// Keep going so one failed batch does not strand the rest.
					im.logger.Error("Failed to upload remaining index data", zap.Error(err), zap.Int("minute", minute))
					continue
				}

				im.logger.Info("Uploaded index batch for the minute", zap.Int("minute", minute))
				delete(im.minuteIndexBatches, minute)
			}
		}
	})
	return nil
}

func (e *enhanceIndexingS3Exporter) start(ctx context.Context, host component.Host) error {
	e.standaloneMode = !e.isHoneycombExtensionPresent(host)

	if e.standaloneMode {
		if e.config.APIEndpoint == "" {
			return fmt.Errorf("api_endpoint is required")
		}

		if err := validateHostname(e.config.APIEndpoint); err != nil {
			return err
		}

		if e.config.APIKey == "" {
			return fmt.Errorf("api_key is required")
		}

		if e.config.APISecret == "" {
			return fmt.Errorf("api_secret is required")
		}

		if e.config.UsageReportingInterval < 30*time.Second {
			return fmt.Errorf("usage_reporting_interval must be at least 30s, got: %s", e.config.UsageReportingInterval)
		}
		if e.config.UsageReportingInterval > 10*time.Minute {
			return fmt.Errorf("usage_reporting_interval must be at most 10m, got: %s", e.config.UsageReportingInterval)
		}

		teamSlug, err := validateAPIKey(e.config)
		if err != nil {
			return fmt.Errorf("failed to validate API credentials: %w", err)
		}
		if teamSlug == "" {
			return fmt.Errorf("team slug is required in standalone mode")
		}
		e.teamSlug = teamSlug
	}

	e.logger.Info("Starting enhance indexing S3 exporter",
		zap.String("region", e.config.S3Uploader.Region),
		zap.String("api_endpoint", e.config.APIEndpoint),
		zap.Bool("standalone_mode", e.standaloneMode),
		zap.String("team_slug", e.teamSlug),
	)

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

	err = e.indexManager.start(ctx, e.s3Writer)
	if err != nil {
		e.logger.Error("Failed to start index manager", zap.Error(err))
		return err
	}

	if e.standaloneMode {
		go e.startMetricsCollection(ctx)
	}

	return nil
}

func (e *enhanceIndexingS3Exporter) shutdown(ctx context.Context) error {
	if e.standaloneMode {
		// Send final metrics before shutdown
		e.collectAndSendMetrics(ctx)
	}

	close(e.done)

	err := e.indexManager.shutdown(ctx)
	if err != nil {
		e.logger.Error("Failed to shutdown index manager", zap.Error(err))
		return err
	}
	return nil
}

// isHoneycombExtensionPresent checks if the honeycombextension is present in the collector
func (e *enhanceIndexingS3Exporter) isHoneycombExtensionPresent(host component.Host) bool {
	if host == nil {
		return false
	}

	extensions := host.GetExtensions()
	for id := range extensions {
		if id.Type().String() == "honeycomb" {
			e.logger.Info("Honeycomb extension detected", zap.String("extension_id", id.String()))
			return true
		}
	}

	return false
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
	currentMinute := time.Now().UTC().Minute()
	im.logger.Info("Timer ticked, checking for index batches to upload", zap.Int("minute", currentMinute))

	// Snapshot and detach ready batches under the write lock so that
	// iteration/mutation of minuteIndexBatches is race-free with the
	// ingestion path (addTracesToIndex / addLogsToIndex / ensureMinuteBatch),
	// which writes under the same lock. Detached batches are then uploaded
	// outside the lock so the slow S3 I/O does not block trace ingestion.
	type readyBatch struct {
		minute int
		batch  *MinuteIndexBatch
	}
	var ready []readyBatch

	im.mutex.Lock()
	for minute, batch := range im.minuteIndexBatches {
		if im.readyToUpload(currentMinute, minute) {
			ready = append(ready, readyBatch{minute: minute, batch: batch})
			delete(im.minuteIndexBatches, minute)
		}
	}
	im.mutex.Unlock()

	for i, rb := range ready {
		im.logger.Info("Index batch is ready to be uploaded", zap.Int("minute", rb.minute))
		if err := im.uploadBatch(ctx, rb.batch); err != nil {
			im.logger.Error("Failed to upload index batch",
				zap.Error(err), zap.Int("minute", rb.minute))
			// Preserve the previous error-handling contract: on a transient
			// upload failure, stop uploading in this cycle and return the
			// failed batch (plus any still-unprocessed ready batches) to the
			// map so the next tick retries them. Skip reinsertion if a fresh
			// batch already exists for the same minute (a >= 1h wraparound),
			// to avoid clobbering new ingest data with stale contents.
			im.mutex.Lock()
			for _, remain := range ready[i:] {
				if _, exists := im.minuteIndexBatches[remain.minute]; !exists {
					im.minuteIndexBatches[remain.minute] = remain.batch
				}
			}
			im.mutex.Unlock()
			break
		}
		im.logger.Info("Uploaded and dropped index batch for the minute", zap.Int("minute", rb.minute))
	}

	// Initialize an empty index batch for the current minute if it doesn't exist
	im.ensureMinuteBatch(currentMinute)
}

// readyToUpload checks if the minute batch is ready to be uploaded
// If the current minute is not equal to the minute of the index batch, the index batch is ready to be uploaded
func (im *IndexManager) readyToUpload(nowMinute, minute int) bool {
	return nowMinute != minute
}

// addTracesToIndex adds trace field information to the current minute's
// MinuteIndexBatch assuming that the field is configured to be indexed. Trace
// ID is always indexed and will be extracted from the span.TraceID().String()
// method. session.id and service.name are also always indexed. Additional
// custom fields are indexed if they are present in configuration. The minute
// passed in comes from the s3Key generated by the s3Writer.WriteBuffer
// function.
func (im *IndexManager) addTracesToIndex(traces ptrace.Traces, s3Key string, minute int) {
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// Ensure the batch exists for this minute and fetch it under the same lock
	// so a concurrent rollover cannot delete it between creation and use.
	currentBatch := im.ensureMinuteBatchLocked(minute)
	currentBatch.minuteDir = filepath.Dir(s3Key)

	// Extract and add field values to the current batch. Every indexed field
	// value found at any level (resource, scope, span) records that this S3 key
	// contains it; the index is inclusive and does not resolve precedence.
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		// Extract Resource attributes
		buildIndexesFromAttributes(currentBatch, rs.Resource().Attributes(), im.config.IndexedFields, s3Key)

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			// Extract Instrumentation scope attributes
			buildIndexesFromAttributes(currentBatch, ss.Scope().Attributes(), im.config.IndexedFields, s3Key)

			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				// Extract span attributes
				buildIndexesFromAttributes(currentBatch, span.Attributes(), im.config.IndexedFields, s3Key)

				// trace id is always indexed from ptrace.Span
				traceID := span.TraceID().String()
				traceIDFName := fieldName("trace.trace_id")
				traceIDFVal := fieldValue(traceID)

				if _, ok := currentBatch.fieldIndexes[traceIDFName]; !ok {
					currentBatch.fieldIndexes[traceIDFName] = map[fieldValue]fieldS3Keys{}
				}

				// Add the S3 key to the trace id field index set
				if currentBatch.fieldIndexes[traceIDFName][traceIDFVal] == nil {
					currentBatch.fieldIndexes[traceIDFName][traceIDFVal] = make(fieldS3Keys)
				}
				currentBatch.fieldIndexes[traceIDFName][traceIDFVal][s3Key] = struct{}{}
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
	im.mutex.Lock()
	defer im.mutex.Unlock()

	// Ensure the batch exists for this minute and fetch it under the same lock
	// so a concurrent rollover cannot delete it between creation and use.
	currentBatch := im.ensureMinuteBatchLocked(minute)
	currentBatch.minuteDir = filepath.Dir(s3Key)

	// Extract and add field values to the current batch. Every indexed field
	// value found at any level (resource, scope, log record) records that this
	// S3 key contains it; the index is inclusive and does not resolve precedence.
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		// Extract Resource attributes
		buildIndexesFromAttributes(currentBatch, rl.Resource().Attributes(), im.config.IndexedFields, s3Key)

		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			// Extract Instrumentation scope attributes
			buildIndexesFromAttributes(currentBatch, sl.Scope().Attributes(), im.config.IndexedFields, s3Key)

			for k := 0; k < sl.LogRecords().Len(); k++ {
				log := sl.LogRecords().At(k)
				// Extract log record attributes
				buildIndexesFromAttributes(currentBatch, log.Attributes(), im.config.IndexedFields, s3Key)

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

					// Add the S3 key to the trace id field index set
					if currentBatch.fieldIndexes[traceIDFName][traceIDFVal] == nil {
						currentBatch.fieldIndexes[traceIDFName][traceIDFVal] = make(fieldS3Keys)
					}
					currentBatch.fieldIndexes[traceIDFName][traceIDFVal][s3Key] = struct{}{}
				}
			}
		}
	}
}

// marshalIndex marshals the index using the configured marshaler type
func (im *IndexManager) marshalIndex(fieldName string, fieldIndex map[fieldValue]fieldS3Keys) ([]byte, error) {
	if im.config.MarshalerName == awss3exporter.OtlpJSON {
		// Convert map[string]struct{} to []string for JSON serialization
		jsonIndex := make(map[string][]string, len(fieldIndex))
		for fv, s3KeysMap := range fieldIndex {
			s3KeysList := make([]string, 0, len(s3KeysMap))
			for s3Key := range s3KeysMap {
				s3KeysList = append(s3KeysList, s3Key)
			}
			jsonIndex[string(fv)] = s3KeysList
		}
		return json.Marshal(jsonIndex)
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
	for fieldVal, s3KeysMap := range fieldIndex {
		s3KeysList := &index.S3Keys{
			S3Keys: make([]string, 0, len(s3KeysMap)),
		}
		for s3Key := range s3KeysMap {
			s3KeysList.S3Keys = append(s3KeysList.S3Keys, s3Key)
		}
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

	// Stamp the processing time onto any span that arrived without a timestamp
	// so it is not stored at the Unix epoch (1970-01-01).
	setMissingSpanTimestamps(traces, pcommon.NewTimestampFromTime(time.Now()))

	// Marshal the traces
	buf, err := e.traceMarshaler.MarshalTraces(traces)
	if err != nil {
		return fmt.Errorf("failed to marshal traces: %w", err)
	}

	// Calculate canonical proto size for logging and usage metrics
	spanBytes := int64(e.traceUsageMarshaler.TracesSize(traces))

	e.logger.Info("Uploading traces",
		zap.Int64("traceSpanCount", spanCount),
		zap.Int64("traceSpanBytes", spanBytes))

	s3Key, minute, err := e.s3Writer.WriteBuffer(ctx, buf, "traces")
	if err != nil {
		return err
	}

	// Add to index batch
	e.indexManager.addTracesToIndex(traces, s3Key, minute)

	// Record usage metrics if in standalone mode
	if e.standaloneMode {
		e.RecordTracesUsage(spanBytes, spanCount)
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

	// Stamp the processing time onto any log record that arrived without a
	// timestamp so it is not stored at the Unix epoch (1970-01-01).
	setMissingLogTimestamps(logs, pcommon.NewTimestampFromTime(time.Now()))

	// Marshal the logs
	buf, err := e.logMarshaler.MarshalLogs(logs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	// Calculate canonical proto size for logging and usage metrics
	logBytes := int64(e.logUsageMarshaler.LogsSize(logs))

	e.logger.Info("Uploading logs",
		zap.Int64("logRecordCount", logCount),
		zap.Int64("logRecordBytes", logBytes))

	s3Key, minute, err := e.s3Writer.WriteBuffer(ctx, buf, "logs")
	if err != nil {
		return err
	}

	// Add to index batch
	e.indexManager.addLogsToIndex(logs, s3Key, minute)

	// Record usage metrics if in standalone mode
	if e.standaloneMode {
		e.RecordLogsUsage(logBytes, logCount)
	}

	return nil
}
