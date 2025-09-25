package enhanceindexings3exporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Mock S3Uploader for testing S3Writer
type mockS3WriterUploader struct {
	uploadCalls []struct {
		input *s3.PutObjectInput
	}
	uploadError  error
	uploadOutput *manager.UploadOutput
}

func (m *mockS3WriterUploader) Upload(ctx context.Context, input *s3.PutObjectInput, opts ...func(*manager.Uploader)) (*manager.UploadOutput, error) {
	m.uploadCalls = append(m.uploadCalls, struct {
		input *s3.PutObjectInput
	}{input})

	if m.uploadError != nil {
		return nil, m.uploadError
	}

	if m.uploadOutput != nil {
		return m.uploadOutput, nil
	}

	return &manager.UploadOutput{}, nil
}

func (m *mockS3WriterUploader) NumCalls() int {
	return len(m.uploadCalls)
}

func TestWriteBuffer(t *testing.T) {
	tests := []struct {
		name              string
		config            *awss3exporter.S3UploaderConfig
		testData          []byte
		signalType        string
		expectedPrefix    string
		checkCustomPrefix bool
		expectCompression bool
	}{
		{
			name: "basic write buffer",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "traces-and-logs/",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			},
			testData:          []byte("test data"),
			signalType:        "traces",
			expectedPrefix:    "traces-and-logs/",
			checkCustomPrefix: false,
			expectCompression: false,
		},
		{
			name: "with gzip compression",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "traces-and-logs/",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				Compression:       "gzip",
			},
			testData:          []byte("test data for compression"),
			signalType:        "traces",
			expectedPrefix:    "traces-and-logs/",
			checkCustomPrefix: false,
			expectCompression: true,
		},
		{
			name: "with custom file prefix",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "traces-and-logs/",
				FilePrefix:        "custom-prefix",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			},
			testData:          []byte("test data"),
			signalType:        "traces",
			expectedPrefix:    "",
			checkCustomPrefix: true,
			expectCompression: false,
		},
		{
			name: "with large data",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "traces-and-logs/",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			},
			testData: func() []byte {
				largeData := make([]byte, 1024*1024) // 1MB
				for i := range largeData {
					largeData[i] = byte(i % 256)
				}
				return largeData
			}(),
			signalType:        "traces",
			expectedPrefix:    "traces-and-logs/",
			checkCustomPrefix: false,
			expectCompression: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			mockUploader := &mockS3WriterUploader{}

			writer := &S3Writer{
				config:    tt.config,
				marshaler: awss3exporter.OtlpJSON,
				uploader:  mockUploader,
				logger:    logger,
			}

			key, minute, err := writer.WriteBuffer(context.Background(), tt.testData, tt.signalType)

			require.NoError(t, err)
			assert.NotEmpty(t, key)
			assert.GreaterOrEqual(t, minute, 0)
			assert.Less(t, minute, 60)

			// Verify the upload call
			uploadCall := mockUploader.uploadCalls[0]
			assert.Equal(t, "test-bucket", *uploadCall.input.Bucket)
			assert.Equal(t, key, *uploadCall.input.Key)

			if tt.expectCompression {
				assert.Equal(t, "gzip", *uploadCall.input.ContentEncoding)

				// Verify the body is compressed
				body := uploadCall.input.Body
				compressedData, err := io.ReadAll(body)
				require.NoError(t, err)

				// Decompress and verify
				gzipReader, err := gzip.NewReader(bytes.NewReader(compressedData))
				require.NoError(t, err)
				defer func() {
					if err := gzipReader.Close(); err != nil {
						t.Logf("Error closing gzip reader: %v", err)
					}
				}()

				decompressedData, err := io.ReadAll(gzipReader)
				require.NoError(t, err)
				assert.Equal(t, tt.testData, decompressedData)
			} else {
				assert.Nil(t, uploadCall.input.ContentEncoding) // No compression

				// Verify the body
				body := uploadCall.input.Body
				uploadedData, err := io.ReadAll(body)
				require.NoError(t, err)
				assert.Equal(t, tt.testData, uploadedData)
			}

			// Additional checks based on test case
			if tt.checkCustomPrefix {
				assert.NotContains(t, key, "custom-prefix_")
			}
			if tt.expectedPrefix != "" {
				assert.Contains(t, key, tt.expectedPrefix)
			}
		})
	}
}

func TestWriteBufferWithIndex(t *testing.T) {
	logger := zap.NewNop()
	config := &awss3exporter.S3UploaderConfig{
		S3Bucket:          "test-bucket",
		Region:            "us-east-1",
		S3Prefix:          "traces-and-logs/",
		S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
	}

	mockUploader := &mockS3WriterUploader{}

	writer := &S3Writer{
		config:    config,
		marshaler: awss3exporter.OtlpJSON,
		uploader:  mockUploader,
		logger:    logger,
	}

	testData := []byte("test data")
	signalType := "traces"
	indexKey := "custom-index-key.json"

	key, minute, err := writer.WriteBufferWithIndex(context.Background(), testData, signalType, indexKey)

	require.NoError(t, err)
	assert.Equal(t, indexKey, key)
	assert.GreaterOrEqual(t, minute, 0)
	assert.Less(t, minute, 60)

	// Verify the upload call
	uploadCall := mockUploader.uploadCalls[0]
	assert.Equal(t, "test-bucket", *uploadCall.input.Bucket)
	assert.Equal(t, indexKey, *uploadCall.input.Key)
	assert.Nil(t, uploadCall.input.ContentEncoding) // No compression

	// Verify the body
	body := uploadCall.input.Body
	uploadedData, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, testData, uploadedData)
}

func TestGenerateKey(t *testing.T) {
	tests := []struct {
		name           string
		config         *awss3exporter.S3UploaderConfig
		marshaler      awss3exporter.MarshalerType
		signalType     string
		expectedPrefix string
		expectedExt    string
		checkPrefix    bool
		checkSlash     bool
	}{
		{
			name: "with prefix and JSON marshaler",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "my-files/",
				FilePrefix:        "traces",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			},
			marshaler:      awss3exporter.OtlpJSON,
			signalType:     "traces",
			expectedPrefix: "my-files/",
			expectedExt:    ".json",
			checkPrefix:    true,
			checkSlash:     false,
		},
		{
			name: "with prefix and Protobuf marshaler",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "my-files/",
				FilePrefix:        "traces",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			},
			marshaler:      awss3exporter.OtlpProtobuf,
			signalType:     "traces",
			expectedPrefix: "my-files/",
			expectedExt:    ".binpb",
			checkPrefix:    true,
			checkSlash:     false,
		},
		{
			name: "with prefix and JSON marshaler with compression",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "my-files/",
				FilePrefix:        "traces",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				Compression:       "gzip",
			},
			marshaler:      awss3exporter.OtlpJSON,
			signalType:     "traces",
			expectedPrefix: "my-files/",
			expectedExt:    ".json.gz",
			checkPrefix:    true,
			checkSlash:     false,
		},
		{
			name: "without prefix",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			},
			marshaler:      awss3exporter.OtlpJSON,
			signalType:     "logs",
			expectedPrefix: "",
			expectedExt:    ".json",
			checkPrefix:    false,
			checkSlash:     true,
		},
		{
			name: "with trailing slash prefix",
			config: &awss3exporter.S3UploaderConfig{
				S3Bucket:          "test-bucket",
				Region:            "us-east-1",
				S3Prefix:          "logs/",
				S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				Compression:       "gzip",
			},
			marshaler:      awss3exporter.OtlpJSON,
			signalType:     "traces",
			expectedPrefix: "logs/",
			expectedExt:    ".json.gz",
			checkPrefix:    true,
			checkSlash:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zap.NewNop()
			mockUploader := &mockS3WriterUploader{}

			writer := &S3Writer{
				config:    tt.config,
				marshaler: tt.marshaler,
				uploader:  mockUploader,
				logger:    logger,
			}

			key, minute := writer.generateKey(tt.signalType)

			// Basic assertions
			assert.NotEmpty(t, key)
			assert.GreaterOrEqual(t, minute, 0)
			assert.Less(t, minute, 60)

			// Verify key structure
			if tt.checkPrefix {
				assert.Contains(t, key, tt.expectedPrefix)
			}
			assert.Contains(t, key, tt.expectedExt)
			assert.Contains(t, key, "year=")
			assert.Contains(t, key, "month=")
			assert.Contains(t, key, "day=")
			assert.Contains(t, key, "hour=")
			assert.Contains(t, key, "minute=")

			// Special checks
			if tt.checkSlash {
				assert.True(t, key[0] != '/', "Should not start with slash")
				assert.Contains(t, key, tt.signalType+"_", "Should use signalType as filePrefix")
			}
		})
	}
}
