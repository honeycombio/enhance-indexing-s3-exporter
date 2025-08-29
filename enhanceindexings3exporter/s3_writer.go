package enhanceindexings3exporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/itchyny/timefmt-go"
	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"go.uber.org/zap"
)

type S3Uploader interface {
	Upload(ctx context.Context, input *s3.PutObjectInput, opts ...func(*manager.Uploader)) (*manager.UploadOutput, error)
}

type S3WriterInterface interface {
	WriteBuffer(ctx context.Context, buf []byte, signalType string) (string, int, error)
	WriteBufferWithIndex(ctx context.Context, buf []byte, signalType string, indexKey string) (string, int, error)
}

type S3Writer struct {
	config    *awss3exporter.S3UploaderConfig
	marshaler awss3exporter.MarshalerType
	uploader  S3Uploader
	logger    *zap.Logger
}

func NewS3Writer(config *awss3exporter.S3UploaderConfig, marshaler awss3exporter.MarshalerType, s3Client *s3.Client, logger *zap.Logger) *S3Writer {
	return &S3Writer{
		config:    config,
		marshaler: marshaler,
		uploader:  manager.NewUploader(s3Client),
		logger:    logger,
	}
}

func (w *S3Writer) WriteBuffer(ctx context.Context, buf []byte, signalType string) (string, int, error) {
	return w.WriteBufferWithIndex(ctx, buf, signalType, "")
}

func (w *S3Writer) WriteBufferWithIndex(ctx context.Context, buf []byte, signalType string, indexKey string) (string, int, error) {
	var key string
	var minute int
	if indexKey != "" {
		key = indexKey
	} else {
		key, minute = w.generateKey(signalType)
	}

	w.logger.Info("Starting S3 upload", zap.String("key", key), zap.String("signalType", signalType), zap.Int("bufferSize", len(buf)))

	var reader io.Reader = bytes.NewReader(buf)

	if w.config.Compression == "gzip" {
		var compressedBuf bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuf)
		if _, err := gzipWriter.Write(buf); err != nil {
			return "", 0, fmt.Errorf("failed to compress data: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return "", 0, fmt.Errorf("failed to close gzip writer: %w", err)
		}
		reader = &compressedBuf
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(w.config.S3Bucket),
		Key:    aws.String(key),
		Body:   reader,
	}

	if w.config.Compression == "gzip" {
		input.ContentEncoding = aws.String("gzip")
	}

	_, err := w.uploader.Upload(ctx, input)
	if err != nil {
		return "", 0, fmt.Errorf("failed to upload to S3: %w", err)
	}

	w.logger.Info("Successfully uploaded to S3", zap.String("key", key), zap.Int("minute", minute))
	return key, minute, nil
}

func (w *S3Writer) generateKey(signalType string) (string, int) {
	prefix := w.config.S3Prefix

	now := time.Now().UTC()

	timePath := timefmt.Format(now, w.config.S3PartitionFormat)

	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}

	filePrefix := w.config.FilePrefix
	if filePrefix == "" {
		filePrefix = signalType
	}

	var marshalerName string
	if w.marshaler == awss3exporter.OtlpJSON {
		marshalerName = "json"
	} else {
		marshalerName = "binpb"
	}

	filename := fmt.Sprintf("%s_%s.%s", filePrefix, uuid.New().String(), marshalerName)
	if w.config.Compression == "gzip" {
		filename += ".gz"
	}

	return fmt.Sprintf("%s%s/%s", prefix, timePath, filename), now.Minute()
}
