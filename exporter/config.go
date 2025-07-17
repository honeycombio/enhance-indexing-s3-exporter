package exporter

import (
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

type Config struct {
	QueueSettings   exporterhelper.QueueSettings   `mapstructure:"sending_queue"`
	TimeoutSettings exporterhelper.TimeoutSettings `mapstructure:",squash"`
	S3Uploader      S3UploaderConfig               `mapstructure:"s3uploader"`
}

type S3UploaderConfig struct {
	Region           string `mapstructure:"region"`
	S3Bucket         string `mapstructure:"s3_bucket"`
	S3Prefix         string `mapstructure:"s3_prefix"`
	S3Partition      string `mapstructure:"s3_partition"`
	FilePrefix       string `mapstructure:"file_prefix"`
	Endpoint         string `mapstructure:"endpoint"`
	S3ForcePathStyle bool   `mapstructure:"s3_force_path_style"`
	DisableSSL       bool   `mapstructure:"disable_ssl"`
	Compression      string `mapstructure:"compression"`
	MaxRetries       int    `mapstructure:"max_retries"`
	RetryMode        string `mapstructure:"retry_mode"`
}

func (c *Config) Validate() error {
	if c.S3Uploader.Region == "" {
		return fmt.Errorf("region is required")
	}
	if c.S3Uploader.S3Bucket == "" && c.S3Uploader.Endpoint == "" {
		return fmt.Errorf("s3_bucket or endpoint is required")
	}

	if c.S3Uploader.Compression != "" && c.S3Uploader.Compression != "gzip" && c.S3Uploader.Compression != "none" {
		return fmt.Errorf("compression must be 'gzip' or 'none', got: %s", c.S3Uploader.Compression)
	}

	if c.S3Uploader.RetryMode != "" && c.S3Uploader.RetryMode != "standard" && c.S3Uploader.RetryMode != "adaptive" {
		return fmt.Errorf("retry_mode must be 'standard' or 'adaptive', got: %s", c.S3Uploader.RetryMode)
	}

	return nil
}

func createDefaultConfig() component.Config {
	return &Config{
		QueueSettings:   exporterhelper.NewDefaultQueueSettings(),
		TimeoutSettings: exporterhelper.NewDefaultTimeoutSettings(),
		S3Uploader: S3UploaderConfig{
			Region:      "us-east-1",
			S3Partition: "minute",
			FilePrefix:  "",
			Compression: "gzip",
			MaxRetries:  3,
			RetryMode:   "standard",
		},
	}
}
