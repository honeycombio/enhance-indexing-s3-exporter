package enhanceindexings3exporter

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

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

	// TeamHCID is extracted from the API response during validation
	TeamHCID string

	// IndexedFields is a list of fields to index.
	IndexedFields []fieldName `mapstructure:"indexed_fields"`
}

func (c *Config) Validate() error {
	if err := validateHostname(c.APIURL); err != nil {
		return err
	}

	if err := validateManagementKey(c.APIURL, string(c.APIKey), c); err != nil {
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

func isLocalAPIURL(apiURL string) bool {
	return strings.Contains(apiURL, "localhost") || strings.Contains(apiURL, "127.0.0.1") || strings.Contains(apiURL, "minio") || strings.Contains(apiURL, "0.0.0.0")
}

func extractTeamHCID(authResp map[string]any) string {
	return authResp["data"].(map[string]any)["relationships"].(map[string]any)["team"].(map[string]any)["data"].(map[string]any)["id"].(string)
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

func validateManagementKey(apiURL string, managementKey string, config *Config) error {

	if (apiURL == "" && managementKey != "") || (apiURL != "" && managementKey == "") {
		return fmt.Errorf("both api_url and management_key must be provided together")
	}

	// Skip API validation for local development
	if isLocalAPIURL(apiURL) {
		fmt.Printf("Skipping API validation for local URL: %s\n", apiURL)
		config.TeamHCID = "local-dev-team" // Set a placeholder team ID for local development
		return nil
	}

	// Construct the auth endpoint URL
	authURL := fmt.Sprintf("%s/2/auth", apiURL)
	// Determine if we are using local config by checking for common local endpoints
	isLocalConfig := isLocalAPIURL(apiURL)

	// Create HTTP client with timeout and skip TLS verification only for local config
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if isLocalConfig {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	// Create request
	req, err := http.NewRequest("GET", authURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	// Set authorization header
	req.Header.Set("Authorization", "Bearer "+managementKey)

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to validate management API key: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	switch resp.StatusCode {
	case http.StatusOK:
		// API key is valid, parse the response to get team ID
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var authResp map[string]any
		if err := json.Unmarshal(body, &authResp); err != nil {
			return fmt.Errorf("failed to parse auth response: %w", err)
		}

		// Store the team HCID in the config
		config.TeamHCID = extractTeamHCID(authResp)
		fmt.Printf("Team HCID extracted and stored: %s\n", config.TeamHCID)

		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("invalid management API key")
	case http.StatusForbidden:
		return fmt.Errorf("management API key does not have required permissions")
	default:
		return fmt.Errorf("unexpected response from auth API: %d", resp.StatusCode)
	}
}
