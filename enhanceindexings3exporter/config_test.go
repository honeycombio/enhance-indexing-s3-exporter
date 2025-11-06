package enhanceindexings3exporter

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func TestConfigMerging(t *testing.T) {
	t.Run("session.id is always in indexed fields", func(t *testing.T) {
		var exp *enhanceIndexingS3Exporter
		createTracesExporter := func(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Traces, error) {
			// Only create the exporter once.
			if exp == nil {
				var err error
				config := cfg.(*Config)
				indexManager := NewIndexManager(config, set.Logger)
				exp, err = newEnhanceIndexingS3Exporter(config, set.Logger, indexManager)
				if err != nil {
					return nil, err
				}
			}

			return exporterhelper.NewTraces(ctx, set, cfg, exp.consumeTraces)
		}

		factory := func() exporter.Factory {
			return exporter.NewFactory(
				component.MustNewType("test"),
				createDefaultConfig,
				exporter.WithTraces(createTracesExporter, stability),
			)
		}

		settings := otelcol.CollectorSettings{
			Factories: func() (factories otelcol.Factories, err error) {
				factories.Exporters = map[component.Type]exporter.Factory{
					factory().Type(): factory(),
				}

				factories.Receivers = map[component.Type]receiver.Factory{
					otlpreceiver.NewFactory().Type(): otlpreceiver.NewFactory(),
				}

				return factories, nil
			},
			ConfigProviderSettings: otelcol.ConfigProviderSettings{
				ResolverSettings: confmap.ResolverSettings{
					URIs: []string{
						"yaml:exporters::test::indexed_fields: [abc]",
						"yaml:exporters::test::api_endpoint: https://api.honeycomb.io",
						"yaml:exporters::test::api_key: test-key",
						"yaml:exporters::test::api_secret: test-secret",
						"yaml:exporters::test::s3uploader::s3_bucket: mybucket",
						"yaml:exporters::test::s3uploader::s3_partition_format: year=%Y/month=%m/day=%d/hour=%H/minute=%M",
						"yaml:exporters::test::s3uploader::compression: gzip",
						"yaml:exporters::test::s3uploader::retry_mode: standard",
						"yaml:exporters::test::s3uploader::retry_max_attempts: 3",
						"yaml:exporters::test::s3uploader::endpoint: http://localhost:9000",
						"yaml:exporters::test::s3uploader::s3_force_path_style: true",
						"yaml:exporters::test::s3uploader::disable_ssl: false",
						"yaml:receivers::otlp::protocols::grpc::endpoint: 0.0.0.0:4317",
						"yaml:service::pipelines::traces/objectstorage::receivers: [otlp]",
						"yaml:service::pipelines::traces/objectstorage::exporters: [test]",
					},
					ProviderFactories: []confmap.ProviderFactory{
						yamlprovider.NewFactory(),
					},
				},
			},
		}

		col, err := otelcol.NewCollector(settings)
		require.NoError(t, err)

		err = col.DryRun(t.Context())
		require.NoError(t, err)

		assert.NotNil(t, exp.config)
		assert.Contains(t, exp.config.IndexedFields, fieldName("session.id"))
	})
}

func TestConfigDefaultQueueBatchConfigValues(t *testing.T) {
	config := createDefaultConfig().(*Config)
	expectedBatchConfig := exporterhelper.BatchConfig{
		MaxSize: 50000,
		Sizer:   exporterhelper.RequestSizerTypeItems,
	}

	assert.Equal(t, config.QueueBatchConfig.Batch.Get().MaxSize, expectedBatchConfig.MaxSize)
	assert.Equal(t, config.QueueBatchConfig.Batch.Get().Sizer, expectedBatchConfig.Sizer)
}

func TestConfigCustomQueueBatchConfigValues(t *testing.T) {
	config := &Config{
		S3Uploader: awss3exporter.S3UploaderConfig{
			Region:            "us-east-1",
			S3Bucket:          "test-bucket",
			S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			Compression:       "gzip",
		},
		MarshalerName: awss3exporter.OtlpProtobuf,
		APIEndpoint:   "https://api.honeycomb.io",
		APIKey:        configopaque.String("test-api-key"),
		APISecret:     configopaque.String("test-api-secret"),
		IndexedFields: []fieldName{"user.id", "service.name"},
		QueueBatchConfig: exporterhelper.QueueBatchConfig{
			Batch: configoptional.Some(exporterhelper.BatchConfig{
				MaxSize: 10_000_000,
				Sizer:   exporterhelper.RequestSizerTypeBytes,
			}),
		},
	}

	expectedBatchConfig := exporterhelper.BatchConfig{
		MaxSize: 10_000_000,
		Sizer:   exporterhelper.RequestSizerTypeBytes,
	}

	err := config.Validate()
	require.NoError(t, err)

	assert.Equal(t, config.QueueBatchConfig.Batch.Get().MaxSize, expectedBatchConfig.MaxSize)
	assert.Equal(t, config.QueueBatchConfig.Batch.Get().Sizer, expectedBatchConfig.Sizer)
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config with all fields",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
					Compression:       "gzip",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
				IndexedFields: []fieldName{"user.id", "service.name"},
			},
			expectError: false,
		},
		{
			name: "valid config with custom endpoint and no bucket",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					Endpoint:          "http://localhost:9000",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpJSON,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: false,
		},
		{
			name: "missing s3_bucket and endpoint",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "s3_bucket is required unless a custom endpoint is provided",
		},
		{
			name: "invalid compression",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
					Compression:       "invalid",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "compression must be 'gzip' or 'none'",
		},
		{
			name: "invalid retry mode",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
					RetryMode:         "invalid",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "retry_mode must be 'standard' or 'adaptive'",
		},
		{
			name: "invalid marshaler",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: "invalid",
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "marshaler must be 'otlp_json' or 'otlp_protobuf'",
		},
		{
			name: "empty s3_partition_format",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "S3PartitionFormat cannot be empty",
		},
		{
			name: "valid s3_partition_format with Go time format",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=2006/month=01/day=02/hour=15/minute=04",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: false,
		},
		{
			name: "invalid s3_partition_format missing placehoders",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "S3PartitionFormat must contain placeholders of year, month, day, hour and minute",
		},
		{
			name: "invalide s3_partition_format start with /",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "/year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "S3PartitionFormat cannot start with '/'",
		},
		{
			name: "invalid s3_partition_format end with /",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M/",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "S3PartitionFormat cannot end with '/'",
		},
		{
			name: "file_prefix is not supported",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
					FilePrefix:        "custom-prefix",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.honeycomb.io",
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
			},
			expectError: true,
			errorMsg:    "file_prefix is not supported",
		},
		{
			name: "hostname missing protocol scheme",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "api.example.com",
				APIKey:        configopaque.String("test-key"),
				APISecret:     configopaque.String("test-secret"),
			},
			expectError: true,
			errorMsg:    "hostname must start with 'http://' or 'https://'",
		},
		{
			name: "invalid hostname too long",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://this-is-a-very-long-hostname-that-exceeds-the-maximum-allowed-length-for-a-hostname-which-is-253-characters-according-to-rfc-standards-and-should-therefore-fail-validation-when-we-test-it-in-our-configuration-validation-tests-to-ensure-that-our-hostname-validation-logic-is-working-correctly-and-properly-rejecting-hostnames-that-are-too-long",
				APIKey:        configopaque.String("test-key"),
				APISecret:     configopaque.String("test-secret"),
			},
			expectError: true,
			errorMsg:    "hostname is too long",
		},
		{
			name: "valid hostname with https",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "https://api.example.com",
				APIKey:        configopaque.String("test-key"),
				APISecret:     configopaque.String("test-secret"),
			},
			expectError: false,
		},
		{
			name: "valid hostname with http",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIEndpoint:   "http://localhost:8086",
				APIKey:        configopaque.String("test-key"),
				APISecret:     configopaque.String("test-secret"),
			},
			expectError: false,
		},
		{
			name: "missing api_endpoint",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String("test-api-secret"),
				APIEndpoint:   "",
			},
			expectError: true,
			errorMsg:    "api_endpoint is required",
		},
		{
			name: "missing api_key",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIKey:        configopaque.String(""),
				APISecret:     configopaque.String("test-api-secret"),
				APIEndpoint:   "https://api.honeycomb.io",
			},
			expectError: true,
			errorMsg:    "api_key is required",
		},
		{
			name: "missing api_secret",
			config: &Config{
				S3Uploader: awss3exporter.S3UploaderConfig{
					Region:            "us-east-1",
					S3Bucket:          "test-bucket",
					S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
				},
				MarshalerName: awss3exporter.OtlpProtobuf,
				APIKey:        configopaque.String("test-api-key"),
				APISecret:     configopaque.String(""),
				APIEndpoint:   "https://api.honeycomb.io",
			},
			expectError: true,
			errorMsg:    "api_secret is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
