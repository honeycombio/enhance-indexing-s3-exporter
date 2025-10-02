package enhanceindexings3exporter

import (
	"fmt"
	"net"
	"strings"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

type Config struct {
	QueueBatchConfig exporterhelper.QueueBatchConfig `mapstructure:"sending_queue"`
	TimeoutConfig    exporterhelper.TimeoutConfig    `mapstructure:",squash"`
	RetryConfig      configretry.BackOffConfig       `mapstructure:"retry_on_failure"`
	S3Uploader       awss3exporter.S3UploaderConfig  `mapstructure:"s3uploader"`
	MarshalerName    awss3exporter.MarshalerType     `mapstructure:"marshaler"`

	// APIKey is the Management API Key associated with the Honeycomb account.
	APIKey configopaque.String `mapstructure:"api_key"`

	// API URL to use (defaults to https://api.honeycomb.io).
	APIURL string `mapstructure:"api_url"`

	// IndexedFields is a list of fields to index.
	IndexedFields []fieldName `mapstructure:"indexed_fields"`
}

func (c *Config) Validate() error {
	if err := validateHostname(c.APIURL); err != nil {
		return err
	}

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

	if c.S3Uploader.FilePrefix != "" {
		return fmt.Errorf("file_prefix is not supported")
	}

	if c.MarshalerName != awss3exporter.OtlpJSON && c.MarshalerName != awss3exporter.OtlpProtobuf {
		return fmt.Errorf("marshaler must be 'otlp_json' or 'otlp_protobuf', got: %s", c.MarshalerName)
	}

	if err := validateS3PartitionFormat(c.S3Uploader.S3PartitionFormat); err != nil {
		return err
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
		APIURL:        "https://api.honeycomb.io",
		APIKey:        configopaque.String(""),
		IndexedFields: []fieldName{},
	}
}

func validateS3PartitionFormat(format string) error {
	if format == "" {
		return fmt.Errorf("S3PartitionFormat cannot be empty")
	}

	goTimePlaceholders := []string{"2006", "01", "02", "15", "04"}
	unixTimePlaceholders := []string{"%Y", "%m", "%d", "%H", "%M"}

	hasGoTimePlaceholders := true

	for _, placeholder := range goTimePlaceholders {
		if !strings.Contains(format, placeholder) {
			hasGoTimePlaceholders = false
			break
		}
	}

	hasUnixTimePlaceholders := true
	for _, placeholder := range unixTimePlaceholders {
		if !strings.Contains(format, placeholder) {
			hasUnixTimePlaceholders = false
			break
		}
	}

	if !hasGoTimePlaceholders && !hasUnixTimePlaceholders {
		return fmt.Errorf("S3PartitionFormat must contain placeholders of year, month, day, hour and minute (e.g., 2006, 01, 02, 15, 04 or %%Y, %%m, %%d, %%H, %%M)")
	}

	if strings.HasPrefix(format, "/") {
		return fmt.Errorf("S3PartitionFormat cannot start with '/'")
	}
	if strings.HasSuffix(format, "/") {
		return fmt.Errorf("S3PartitionFormat cannot end with '/'")
	}
	return nil
}

func validateHostname(hostname string) error {
	if hostname == "" {
		// Hostname is optional for now, so empty string is valid
		return nil
	}

	// Check if it's a valid IP address
	if net.ParseIP(hostname) != nil {
		return nil
	}

	// Check if it's a valid hostname/FQDN
	if len(hostname) > 253 {
		return fmt.Errorf("hostname is too long: %s", hostname)
	}

	return nil
}
