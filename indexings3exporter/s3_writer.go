package exporter

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

type S3Writer struct {
	config   *awss3exporter.S3UploaderConfig
	uploader *manager.Uploader
	logger   *zap.Logger
}

func NewS3Writer(config *awss3exporter.S3UploaderConfig, s3Client *s3.Client, logger *zap.Logger) *S3Writer {
	return &S3Writer{
		config:   config,
		uploader: manager.NewUploader(s3Client),
		logger:   logger,
	}
}

func (w *S3Writer) WriteBuffer(ctx context.Context, buf []byte, signalType string) error {
	key := w.generateKey(signalType)

	var reader io.Reader = bytes.NewReader(buf)

	if w.config.Compression == "gzip" {
		var compressedBuf bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuf)
		if _, err := gzipWriter.Write(buf); err != nil {
			return fmt.Errorf("failed to compress data: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %w", err)
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
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	w.logger.Debug("Successfully uploaded to S3", zap.String("key", key))
	return nil
}

func (w *S3Writer) generateKey(signalType string) string {
	now := time.Now()

	timePath := timefmt.Format(now, w.config.S3PartitionFormat)

	prefix := w.config.S3Prefix
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}

	filePrefix := w.config.FilePrefix
	if filePrefix == "" {
		filePrefix = signalType
	}

	filename := fmt.Sprintf("%s-%s.json", filePrefix, uuid.New().String())
	if w.config.Compression == "gzip" {
		filename += ".gz"
	}

	return fmt.Sprintf("%s%s/%s", prefix, timePath, filename)
}
