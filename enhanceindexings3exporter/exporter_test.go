package enhanceindexings3exporter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/honeycombio/enhance-indexing-s3-exporter/index"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// Mock S3Uploader for testing S3Writer
type mockS3Uploader struct {
	uploadCalls []struct {
		input *s3.PutObjectInput
		opts  []func(*manager.Uploader)
	}
	uploadError  error
	uploadOutput *manager.UploadOutput
}

func (m *mockS3Uploader) Upload(ctx context.Context, input *s3.PutObjectInput, opts ...func(*manager.Uploader)) (*manager.UploadOutput, error) {
	m.uploadCalls = append(m.uploadCalls, struct {
		input *s3.PutObjectInput
		opts  []func(*manager.Uploader)
	}{input, opts})

	if m.uploadError != nil {
		return nil, m.uploadError
	}

	if m.uploadOutput != nil {
		return m.uploadOutput, nil
	}

	return &manager.UploadOutput{}, nil
}

func TestConsumeTraces(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		traces ptrace.Traces
	}{
		{
			name: "traces with indexing",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:   "us-east-1",
					S3Bucket: "test-bucket",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				IndexedFields: []fieldName{"user.id"},
			},
			traces: createTestTraces(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			indexManager := NewIndexManager(tt.config, logger)
			exporter, err := newEnhanceIndexingS3Exporter(ctx, tt.config, logger, indexManager)
			require.NoError(t, err)

			// Mock the s3Writer
			mockWriter := createMockS3Writer(&tt.config.S3Uploader, tt.config.MarshalerName, logger)
			exporter.s3Writer = mockWriter
			if exporter.indexManager != nil {
				exporter.indexManager.s3Writer = mockWriter
			}
			mockUploader := mockWriter.uploader.(*mockS3Uploader)
			// Get the current minute to match what WriteBuffer will return
			currentMinute := time.Now().UTC().Minute()
			exporter.indexManager.minuteIndexBatches = map[int]*MinuteIndexBatch{
				currentMinute: {
					fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
				},
			}

			ctx := context.Background()
			err = exporter.consumeTraces(ctx, tt.traces)

			require.NoError(t, err)

			assert.Len(t, mockUploader.uploadCalls, 1)
			assert.Contains(t, *mockUploader.uploadCalls[0].input.Key, "traces_")

			// Check that index was updated
			currentMinute = time.Now().UTC().Minute()
			batch := exporter.indexManager.minuteIndexBatches[currentMinute]
			assert.NotNil(t, batch)

			// Check that trace.trace_id and session.id are indexed automatically
			assert.Contains(t, batch.fieldIndexes[fieldName("trace.trace_id")], fieldValue("00000000000000000000000000000001"))
			assert.Contains(t, batch.fieldIndexes[fieldName("session.id")], fieldValue("12345"))
			assert.Contains(t, batch.fieldIndexes[fieldName("service.name")], fieldValue("test-service"))

			// Check that configured fields are indexed
			assert.Contains(t, batch.fieldIndexes[fieldName("user.id")], fieldValue("user123"))

			// Check that non-configured fields are not indexed
			assert.NotContains(t, batch.fieldIndexes, fieldName("request.id"))

			// Clean up after checking
			err = exporter.shutdown(ctx)
			require.NoError(t, err)
		})
	}
}

func TestConsumeLogs(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		logs           plog.Logs
		expectIndexing bool
	}{
		{
			name: "logs with indexing",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:   "us-east-1",
					S3Bucket: "test-bucket",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				IndexedFields: []fieldName{"customer.id"},
			},
			logs:           createTestLogs(),
			expectIndexing: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			indexManager := NewIndexManager(tt.config, logger)
			exporter, err := newEnhanceIndexingS3Exporter(ctx, tt.config, logger, indexManager)
			require.NoError(t, err)

			// Mock the s3Writer
			mockWriter := createMockS3Writer(&tt.config.S3Uploader, tt.config.MarshalerName, logger)
			exporter.s3Writer = mockWriter
			if exporter.indexManager != nil {
				exporter.indexManager.s3Writer = mockWriter
			}
			mockUploader := mockWriter.uploader.(*mockS3Uploader)
			// Get the current minute to match what WriteBuffer will return
			currentMinute := time.Now().UTC().Minute()
			exporter.indexManager.minuteIndexBatches = map[int]*MinuteIndexBatch{
				currentMinute: {
					fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
				},
			}

			ctx := context.Background()
			err = exporter.consumeLogs(ctx, tt.logs)

			require.NoError(t, err)

			assert.Len(t, mockUploader.uploadCalls, 1)
			assert.Contains(t, *mockUploader.uploadCalls[0].input.Key, "logs_")

			if tt.expectIndexing {
				// Check that index was updated
				currentMinute := time.Now().UTC().Minute()
				batch := exporter.indexManager.minuteIndexBatches[currentMinute]
				assert.NotNil(t, batch)

				// Check that trace.trace_id and session.id are indexed automatically
				assert.Contains(t, batch.fieldIndexes[fieldName("trace.trace_id")], fieldValue("00000000000000000000000000000001"))
				assert.Contains(t, batch.fieldIndexes[fieldName("session.id")], fieldValue("12345"))
				assert.Contains(t, batch.fieldIndexes[fieldName("service.name")], fieldValue("test-service"))

				// Check that configured fields are indexed
				assert.Contains(t, batch.fieldIndexes[fieldName("customer.id")], fieldValue("cust123"))

				// Check that non-configured fields are not indexed
				assert.NotContains(t, batch.fieldIndexes, fieldName("request.id"))
			}
			// Clean up after checking
			err = exporter.shutdown(ctx)
			require.NoError(t, err)
		})
	}
}

func TestAddTracesToIndex(t *testing.T) {
	logger := zap.NewNop()
	config := &Config{
		IndexedFields: []fieldName{"user.id"},
	}

	indexManager := NewIndexManager(config, logger)
	exporter, err := newEnhanceIndexingS3Exporter(ctx, config, logger, indexManager)
	require.NoError(t, err)

	// Initialize index batch
	exporter.indexManager.minuteIndexBatches = map[int]*MinuteIndexBatch{
		30: {
			fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
		},
	}

	traces := createTestTraces()
	s3Key := "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=30/traces_uuid.binpb.gz"
	minute := 30

	exporter.indexManager.addTracesToIndex(traces, s3Key, minute)

	// Verify index was updated correctly
	batch := exporter.indexManager.minuteIndexBatches[minute]
	assert.Equal(t, "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=30", batch.minuteDir)

	// Check trace.trace_id indexing
	traceID := "00000000000000000000000000000001"
	assert.Contains(t, batch.fieldIndexes[fieldName("trace.trace_id")], fieldValue(traceID))
	assert.Contains(t, batch.fieldIndexes[fieldName("trace.trace_id")][fieldValue(traceID)], s3Key)

	// Check session.id indexing (automatically included when indexing is enabled)
	assert.Contains(t, batch.fieldIndexes[fieldName("session.id")], fieldValue("12345"))
	assert.Contains(t, batch.fieldIndexes[fieldName("session.id")][fieldValue("12345")], s3Key)

	// Check service.name indexing (automatically included when indexing is enabled)
	assert.Contains(t, batch.fieldIndexes[fieldName("service.name")], fieldValue("test-service"))
	assert.Contains(t, batch.fieldIndexes[fieldName("service.name")][fieldValue("test-service")], s3Key)

	// Check configured field indexing
	assert.Contains(t, batch.fieldIndexes[fieldName("user.id")], fieldValue("user123"))
	assert.Contains(t, batch.fieldIndexes[fieldName("user.id")][fieldValue("user123")], s3Key)

	// Check that attribute precedence is respected, Item > Scope > Resource
	assert.NotContains(t, batch.fieldIndexes[fieldName("user.id")], fieldValue("user456"))
	assert.NotContains(t, batch.fieldIndexes[fieldName("user.id")], fieldValue("user789"))

	// Check that non-configured fields are not indexed
	assert.NotContains(t, batch.fieldIndexes, fieldName("request.id"))
}

func TestAddLogsToIndex(t *testing.T) {
	logger := zap.NewNop()
	config := &Config{
		IndexedFields: []fieldName{"customer.id"},
	}

	indexManager := NewIndexManager(config, logger)
	exporter, err := newEnhanceIndexingS3Exporter(ctx, config, logger, indexManager)
	require.NoError(t, err)

	// Initialize index batch
	exporter.indexManager.minuteIndexBatches = map[int]*MinuteIndexBatch{
		30: {
			fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
		},
	}

	logs := createTestLogs()
	s3Key := "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=30/traces_uuid.binpb.gz"
	minute := 30

	exporter.indexManager.addLogsToIndex(logs, s3Key, minute)

	// Verify index was updated correctly
	batch := exporter.indexManager.minuteIndexBatches[minute]
	assert.Equal(t, "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=30", batch.minuteDir)

	// Check trace.trace_id indexing
	assert.Contains(t, batch.fieldIndexes[fieldName("trace.trace_id")], fieldValue("00000000000000000000000000000001"))
	assert.Contains(t, batch.fieldIndexes[fieldName("trace.trace_id")][fieldValue("00000000000000000000000000000001")], s3Key)

	// Check session.id indexing (automatically included when indexing is enabled)
	assert.Contains(t, batch.fieldIndexes[fieldName("session.id")], fieldValue("12345"))
	assert.Contains(t, batch.fieldIndexes[fieldName("session.id")][fieldValue("12345")], s3Key)

	// Check service.name indexing (automatically included when indexing is enabled)
	assert.Contains(t, batch.fieldIndexes[fieldName("service.name")], fieldValue("test-service"))
	assert.Contains(t, batch.fieldIndexes[fieldName("service.name")][fieldValue("test-service")], s3Key)

	// Check configured field indexing
	assert.Contains(t, batch.fieldIndexes[fieldName("customer.id")], fieldValue("cust123"))
	assert.Contains(t, batch.fieldIndexes[fieldName("customer.id")][fieldValue("cust123")], s3Key)

	// Check that attribute precedence is respected, Item > Scope > Resource
	assert.NotContains(t, batch.fieldIndexes[fieldName("customer.id")], fieldValue("cust456"))
	assert.NotContains(t, batch.fieldIndexes[fieldName("customer.id")], fieldValue("cust789"))

	// Check that non-configured fields are not indexed
	assert.NotContains(t, batch.fieldIndexes, fieldName("request.id"))

}

func TestMarshalIndex(t *testing.T) {
	tests := []struct {
		name          string
		marshalerName awss3exporter.MarshalerType
		fieldName     string
		fieldIndex    map[fieldValue]fieldS3Keys
		expectError   bool
	}{
		{
			name:          "JSON marshaler",
			marshalerName: awss3exporter.OtlpJSON,
			fieldName:     "user.id",
			fieldIndex: map[fieldValue]fieldS3Keys{
				"user123": {"key1", "key2"},
				"user456": {"key3"},
			},
			expectError: false,
		},
		{
			name:          "Protobuf marshaler",
			marshalerName: awss3exporter.OtlpProtobuf,
			fieldName:     "user.id",
			fieldIndex: map[fieldValue]fieldS3Keys{
				"user123": {"key1", "key2"},
				"user456": {"key3"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			config := &Config{
				MarshalerName: tt.marshalerName,
			}

			indexManager := NewIndexManager(config, logger)
			exporter, err := newEnhanceIndexingS3Exporter(ctx, config, logger, indexManager)
			require.NoError(t, err)

			data, err := exporter.indexManager.marshalIndex(tt.fieldName, tt.fieldIndex)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, data)

			// Validate with unmarshalling
			switch tt.marshalerName {
			case awss3exporter.OtlpJSON:
				var unmarshaled map[string]interface{}
				err = json.Unmarshal(data, &unmarshaled)
				assert.NoError(t, err)
			case awss3exporter.OtlpProtobuf:
				fieldIndex := &index.FieldIndex{}
				err = fieldIndex.Unmarshal(data)
				assert.NoError(t, err)
			}
		})
	}
}

func TestUploadBatch(t *testing.T) {
	logger := zap.NewNop()
	config := &Config{
		MarshalerName: awss3exporter.OtlpJSON,
		S3Uploader: awss3exporter.S3UploaderConfig{
			Compression: "gzip",
		},
	}

	indexManager := NewIndexManager(config, logger)
	exporter, err := newEnhanceIndexingS3Exporter(ctx, config, logger, indexManager)
	require.NoError(t, err)

	// Mock the s3Writer
	mockWriter := createMockS3Writer(&config.S3Uploader, config.MarshalerName, logger)
	exporter.s3Writer = mockWriter
	if exporter.indexManager != nil {
		exporter.indexManager.s3Writer = mockWriter
	}
	mockUploader := mockWriter.uploader.(*mockS3Uploader)
	// Create test batch
	batch := &MinuteIndexBatch{
		minuteDir: "traces-and-logs/year=2025/month=07/day=28/hour=12/minute=30",
		fieldIndexes: map[fieldName]map[fieldValue]fieldS3Keys{
			"user.id": {
				"user123": {"key1", "key2"},
				"user456": {"key3"},
			},
			"request.id": {
				"req789": {"key4"},
			},
			"session.id": {
				"12345": {"key5"},
			},
			"trace.trace_id": {
				"00000000000000000000000000000001": {"key6"},
			},
		},
	}

	ctx := context.Background()
	err = exporter.indexManager.uploadBatch(ctx, batch)

	require.NoError(t, err)

	assert.Len(t, mockUploader.uploadCalls, 4) // user.id, request.id, session.id, trace.trace_id

	for _, call := range mockUploader.uploadCalls {
		assert.NotNil(t, call.input)
		assert.Contains(t, *call.input.Key, "index_")
	}
}

func TestRolloverIndexes(t *testing.T) {
	logger := zap.NewNop()
	config := &Config{
		IndexedFields: []fieldName{"user.id"},
	}

	indexManager := NewIndexManager(config, logger)
	exporter, err := newEnhanceIndexingS3Exporter(ctx, config, logger, indexManager)
	require.NoError(t, err)

	// Mock the s3Writer
	mockWriter := createMockS3Writer(&config.S3Uploader, config.MarshalerName, logger)
	exporter.s3Writer = mockWriter
	if exporter.indexManager != nil {
		exporter.indexManager.s3Writer = mockWriter
	}
	mockUploader := mockWriter.uploader.(*mockS3Uploader)

	// Initialize with old minute batch
	oldMinute := (time.Now().UTC().Minute() + 1) % 60
	exporter.indexManager.minuteIndexBatches = map[int]*MinuteIndexBatch{
		oldMinute: {
			fieldIndexes: map[fieldName]map[fieldValue]fieldS3Keys{
				"user.id": {
					"user123": {"key1"},
				},
			},
		},
	}

	// Call rollover
	ctx := context.Background()
	exporter.indexManager.rolloverIndexes(ctx)

	// Verify that old batch was uploaded and removed
	assert.Len(t, mockUploader.uploadCalls, 1)
	assert.Empty(t, exporter.indexManager.minuteIndexBatches[oldMinute])

	// Verify new batch was created for current minute
	currentMinute := time.Now().UTC().Minute()
	assert.NotNil(t, exporter.indexManager.minuteIndexBatches[currentMinute])
}

func createMockS3Writer(config *awss3exporter.S3UploaderConfig, marshaler awss3exporter.MarshalerType, logger *zap.Logger) *S3Writer {
	return &S3Writer{
		config:    config,
		marshaler: marshaler,
		uploader:  &mockS3Uploader{},
		logger:    logger,
	}
}

// Helper functions to create test data
func createTestTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("user.id", "user456")

	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().Attributes().PutStr("user.id", "user789")

	span := ss.Spans().AppendEmpty()

	traceID := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	span.SetTraceID(traceID)

	span.Attributes().PutStr("user.id", "user123")
	span.Attributes().PutStr("request.id", "req456")
	span.Attributes().PutInt("session.id", 12345)
	span.Attributes().PutStr("service.name", "test-service")

	return traces
}

func createTestLogs() plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("customer.id", "cust456")

	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().Attributes().PutStr("customer.id", "cust789")

	lr := sl.LogRecords().AppendEmpty()

	traceID := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	lr.SetTraceID(traceID)

	lr.Body().SetStr("test log message")

	lr.Attributes().PutStr("level", "info")
	lr.Attributes().PutStr("customer.id", "cust123")
	lr.Attributes().PutStr("service", "test-service")
	lr.Attributes().PutInt("session.id", 12345)
	lr.Attributes().PutStr("service.name", "test-service")

	return logs
}
