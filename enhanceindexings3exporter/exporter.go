package enhanceindexings3exporter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

type enhanceIndexingS3Exporter struct {
	config       *Config
	logger       *zap.Logger
	s3Writer     *S3Writer
	indexBuilder *IndexBuilder
}

func newEnhanceIndexingS3Exporter(cfg *Config, logger *zap.Logger) (*enhanceIndexingS3Exporter, error) {
	return &enhanceIndexingS3Exporter{
		config: cfg,
		logger: logger,
	}, nil
}

func (e *enhanceIndexingS3Exporter) start(ctx context.Context, host component.Host) error {
	e.logger.Info("Starting enhance indexing S3 exporter", zap.String("region", e.config.S3Uploader.Region))

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
		return fmt.Errorf("S3 bucket name is empty")
	}

	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})

	// Continue if the bucket exists
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() != "BucketAlreadyOwnedByYou" && apiErr.ErrorCode() != "BucketAlreadyExists" {
				return fmt.Errorf("failed to create bucket: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	e.s3Writer = NewS3Writer(&e.config.S3Uploader, e.config.MarshalerName, s3Client, e.logger)

	// Initialize index builder
	e.indexBuilder = NewIndexBuilder()

	return nil
}

func (e *enhanceIndexingS3Exporter) shutdown(ctx context.Context) error {
	return nil
}

func (e *enhanceIndexingS3Exporter) uploadIndex(ctx context.Context, indexData []IndexEntry) error {
	e.logger.Info("Uploading index")
	if len(indexData) == 0 {
		e.logger.Info("No index data to upload")
		return nil
	}

	index, err := json.Marshal(indexData)
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	return e.s3Writer.WriteBuffer(ctx, index, "index")
}

func (e *enhanceIndexingS3Exporter) extractIndexFields(traces ptrace.Traces, s3Location string) []IndexEntry {
	var entries []IndexEntry

	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)

		// Extract service name
		if serviceName, ok := rs.Resource().Attributes().Get("service.name"); ok {
			entries = append(entries, IndexEntry{
				FieldName:  "service.name",
				FieldValue: serviceName.AsString(),
				S3Location: s3Location,
				SignalType: "traces",
				Timestamp:  time.Now(),
				Count:      1,
			})
		}

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)

				// Extract span name
				entries = append(entries, IndexEntry{
					FieldName:  "span.name",
					FieldValue: span.Name(),
					S3Location: s3Location,
					SignalType: "traces",
					Timestamp:  time.Now(),
					Count:      1,
				})

				// Extract trace ID
				entries = append(entries, IndexEntry{
					FieldName:  "trace.id",
					FieldValue: span.TraceID().String(),
					S3Location: s3Location,
					SignalType: "traces",
					Timestamp:  time.Now(),
					Count:      1,
				})

				// Extract custom attributes based on configuration
				span.Attributes().Range(func(k string, v pcommon.Value) bool {
					if e.config.IndexConfig.Enabled && slices.Contains(e.config.IndexConfig.IndexedFields, k) {
						entries = append(entries, IndexEntry{
							FieldName:  k,
							FieldValue: v.AsString(),
							S3Location: s3Location,
							SignalType: "traces",
							Timestamp:  time.Now(),
							Count:      1,
						})
					}
					return true
				})
			}
		}
	}

	return entries
}

func (e *enhanceIndexingS3Exporter) consumeTraces(ctx context.Context, traces ptrace.Traces) error {
	e.logger.Info("Consuming traces", zap.Int("spanCount", traces.SpanCount()))

	var marshaler ptrace.Marshaler
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		marshaler = &ptrace.JSONMarshaler{}
	} else {
		marshaler = &ptrace.ProtoMarshaler{}
	}

	buf, err := marshaler.MarshalTraces(traces)
	if err != nil {
		return fmt.Errorf("failed to marshal traces: %w", err)
	}

	// Extract index entries and prepare index for upload if enabled
	var indexData []IndexEntry
	if e.config.IndexConfig.Enabled {
		indexData = e.extractIndexFields(traces, e.config.S3Uploader.S3Prefix)
		e.indexBuilder.AddEntries(indexData)
		if err := e.uploadIndex(ctx, indexData); err != nil {
			e.logger.Error("failed to upload index", zap.Error(err))
		}
	}

	e.logger.Info("Uploading traces", zap.Int("traceSpanCount", traces.SpanCount()))
	return e.s3Writer.WriteBuffer(ctx, buf, "traces")
}

func (e *enhanceIndexingS3Exporter) consumeLogs(ctx context.Context, logs plog.Logs) error {
	e.logger.Info("Consuming logs", zap.Int("logRecordCount", logs.LogRecordCount()))

	// TODO: Add log fields to index

	var marshaler plog.Marshaler
	if e.config.MarshalerName == awss3exporter.OtlpJSON {
		marshaler = &plog.JSONMarshaler{}
	} else {
		marshaler = &plog.ProtoMarshaler{}
	}

	buf, err := marshaler.MarshalLogs(logs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	e.logger.Info("Uploading logs", zap.Int("logRecordCount", logs.LogRecordCount()))
	return e.s3Writer.WriteBuffer(ctx, buf, "logs")
}

type IndexEntry struct {
	FieldName  string    `json:"field_name"`
	FieldValue string    `json:"field_value"`
	S3Location string    `json:"s3_location"`
	SignalType string    `json:"signal_type"`
	Timestamp  time.Time `json:"timestamp"`
	Count      int       `json:"count"`
}

type IndexBuilder struct {
	entries map[string]*IndexEntry
	mutex   sync.RWMutex
}

func NewIndexBuilder() *IndexBuilder {
	return &IndexBuilder{
		entries: make(map[string]*IndexEntry),
	}
}

func (b *IndexBuilder) AddEntries(entries []IndexEntry) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for _, entry := range entries {
		b.entries[entry.FieldName] = &entry
	}
}
