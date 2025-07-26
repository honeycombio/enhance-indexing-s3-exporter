package enhanceindexings3exporter

import (
	"fmt"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

type Config struct {
	QueueBatchConfig exporterhelper.QueueBatchConfig `mapstructure:"sending_queue"`
	TimeoutConfig    exporterhelper.TimeoutConfig    `mapstructure:",squash"`
	RetryConfig      configretry.BackOffConfig       `mapstructure:"retry_on_failure"`
	S3Uploader       awss3exporter.S3UploaderConfig  `mapstructure:"s3uploader"`
	MarshalerName    awss3exporter.MarshalerType     `mapstructure:"marshaler"`

	// Index configuration
	IndexConfig IndexConfig `mapstructure:"index"`
}

type IndexConfig struct {
	Enabled       bool     `mapstructure:"enabled"`
	IndexedFields []string `mapstructure:"indexed_fields"`
}

func (c *Config) Validate() error {
	if c.S3Uploader.Region == "" {
		return fmt.Errorf("region is required")
	}
	if c.S3Uploader.S3Bucket == "" && c.S3Uploader.Endpoint == "" {
		return fmt.Errorf("s3_bucket is required unless a custom endpoint is provided")
	}

	if c.S3Uploader.Compression != "" && c.S3Uploader.Compression != "gzip" && c.S3Uploader.Compression != "none" {
		return fmt.Errorf("compression must be 'gzip' or 'none', got: %s", c.S3Uploader.Compression)
	}

	if c.S3Uploader.RetryMode != "" && c.S3Uploader.RetryMode != "standard" && c.S3Uploader.RetryMode != "adaptive" {
		return fmt.Errorf("retry_mode must be 'standard' or 'adaptive', got: %s", c.S3Uploader.RetryMode)
	}

	if c.MarshalerName != awss3exporter.OtlpJSON && c.MarshalerName != awss3exporter.OtlpProtobuf {
		return fmt.Errorf("marshaler must be 'otlp_json' or 'otlp_protobuf', got: %s", c.MarshalerName)
	}

	return nil
}

func createDefaultConfig() component.Config {
	return &Config{
		QueueBatchConfig: exporterhelper.NewDefaultQueueConfig(),
		TimeoutConfig:    exporterhelper.NewDefaultTimeoutConfig(),
		RetryConfig:      configretry.NewDefaultBackOffConfig(),
		S3Uploader: awss3exporter.S3UploaderConfig{
			Region:            "us-east-1",
			S3PartitionFormat: "year=%Y/month=%m/day=%d/hour=%H/minute=%M",
			FilePrefix:        "",
			Compression:       "gzip",
			RetryMaxAttempts:  3,
			RetryMode:         "standard",
		},
		MarshalerName: awss3exporter.OtlpProtobuf,
		IndexConfig: IndexConfig{
			Enabled:       false,
			IndexedFields: []string{},
		},
	}
}
