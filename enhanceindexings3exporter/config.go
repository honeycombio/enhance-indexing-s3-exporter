package enhanceindexings3exporter

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/config/configoptional"
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
	// APISecret is the Management API Secret associated with the Honeycomb account.
	APIKey    configopaque.String `mapstructure:"api_key"`
	APISecret configopaque.String `mapstructure:"api_secret"`
	// APIEndpoint is the Honeycomb API endpoint
	APIEndpoint string `mapstructure:"api_endpoint"`

	// IndexedFields is a list of fields to index.
	IndexedFields []fieldName `mapstructure:"indexed_fields"`
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

	if c.S3Uploader.FilePrefix != "" {
		return fmt.Errorf("file_prefix is not supported")
	}

	if c.MarshalerName != awss3exporter.OtlpJSON && c.MarshalerName != awss3exporter.OtlpProtobuf {
		return fmt.Errorf("marshaler must be 'otlp_json' or 'otlp_protobuf', got: %s", c.MarshalerName)
	}

	if err := validateS3PartitionFormat(c.S3Uploader.S3PartitionFormat); err != nil {
		return err
	}

	if c.APIEndpoint == "" {
		return fmt.Errorf("api_endpoint is required")
	}

	if err := validateHostname(c.APIEndpoint); err != nil {
		return err
	}

	if c.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}

	if c.APISecret == "" {
		return fmt.Errorf("api_secret is required")
	}

	return nil
}

func createDefaultConfig() component.Config {
	queueConfig := exporterhelper.NewDefaultQueueConfig()

	// Set a sensible default max size and sizer for the queue batch if not configured
	queueConfig.Batch = configoptional.Some(exporterhelper.BatchConfig{
		MaxSize: 50000,
		Sizer:   exporterhelper.RequestSizerTypeItems,
	})

	return &Config{
		QueueBatchConfig: queueConfig,
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

	if !strings.HasPrefix(hostname, "http://") && !strings.HasPrefix(hostname, "https://") {
		return fmt.Errorf("hostname must start with 'http://' or 'https://', got: %s", hostname)
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

var authResponse struct {
	Data struct {
		Attributes struct {
			Disabled bool     `json:"disabled"`
			Scopes   []string `json:"scopes"`
		} `json:"attributes"`
	}
	Included []struct {
		Attributes struct {
			Slug string `json:"slug"`
		} `json:"attributes"`
	} `json:"included"`
}

func validateAPIKey(cfg *Config) (string, error) {
	authURL := fmt.Sprintf("%s/2/auth", cfg.APIEndpoint)

	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+string(cfg.APIKey)+":"+string(cfg.APISecret))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to validate API key: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}

		if err := json.Unmarshal(body, &authResponse); err != nil {
			return "", fmt.Errorf("failed to parse auth response: %w", err)
		}

		if authResponse.Data.Attributes.Disabled {
			return "", fmt.Errorf("API key is disabled")
		}
		if !slices.Contains(authResponse.Data.Attributes.Scopes, "enhance:write") {
			return "", fmt.Errorf("API key does not have the required scopes")
		}

		if len(authResponse.Included) == 0 || authResponse.Included[0].Attributes.Slug == "" {
			return "", fmt.Errorf("auth response did not contain valid team slug")
		}

		return authResponse.Included[0].Attributes.Slug, nil
	case http.StatusUnauthorized:
		return "", fmt.Errorf("invalid API key")
	case http.StatusForbidden:
		return "", fmt.Errorf("API key does not have required permissions")
	default:
		return "", fmt.Errorf("unexpected response from auth API: %d", resp.StatusCode)
	}
}
