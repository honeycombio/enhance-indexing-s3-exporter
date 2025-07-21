package exporter

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type enhanceIndexingS3Exporter struct {
	config   *Config
	s3Writer *S3Writer
	logger   *zap.Logger
}

func newEnhanceIndexingS3Exporter(cfg *Config, logger *zap.Logger) (*enhanceIndexingS3Exporter, error) {
	return &enhanceIndexingS3Exporter{
		config: cfg,
		logger: logger,
	}, nil
}

func (e *enhanceIndexingS3Exporter) start(ctx context.Context, host component.Host) error {
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

	e.s3Writer = NewS3Writer(&e.config.S3Uploader, s3Client, e.logger)
	return nil
}

func (e *enhanceIndexingS3Exporter) shutdown(ctx context.Context) error {
	return nil
}

func (e *enhanceIndexingS3Exporter) consumeTraces(ctx context.Context, traces ptrace.Traces) error {
	marshaler := &ptrace.JSONMarshaler{}
	buf, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return fmt.Errorf("failed to marshal traces: %w", err)
	}

	// TODO: Encode the traces before writing to S3
	// using the configured Config.MarshalerName

	return e.s3Writer.WriteBuffer(ctx, buf, "traces")
}

func (e *enhanceIndexingS3Exporter) consumeLogs(ctx context.Context, logs plog.Logs) error {
	marshaler := &plog.JSONMarshaler{}
	buf, err := marshaler.MarshalLogs(logs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	// TODO: Encode the logs before writing to S3
	// using the configured Config.MarshalerName

	return e.s3Writer.WriteBuffer(ctx, buf, "logs")
}

type EnhancedData struct {
	OriginalData interface{} `json:"original_data"`
	Metadata     struct {
		Timestamp   string `json:"timestamp"`
		SignalType  string `json:"signal_type"`
		Environment string `json:"environment,omitempty"`
		Service     string `json:"service,omitempty"`
	} `json:"metadata"`
	IndexingHints struct {
		PrimaryKeys      []string `json:"primary_keys,omitempty"`
		SearchableFields []string `json:"searchable_fields,omitempty"`
		Categories       []string `json:"categories,omitempty"`
	} `json:"indexing_hints"`
}
