package exporter

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	typeStr   = "enhance-indexing-s3-exporter"
	stability = component.StabilityLevelAlpha
)

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		typeStr,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, stability),
		exporter.WithLogs(createLogsExporter, stability),
	)
}

func createTracesExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	cfg component.Config,
) (exporter.Traces, error) {
	config := cfg.(*Config)

	s3Exporter, err := newEnhanceIndexingS3Exporter(config, set.Logger)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewTracesExporter(
		ctx,
		set,
		cfg,
		s3Exporter.consumeTraces,
		exporterhelper.WithStart(s3Exporter.start),
		exporterhelper.WithShutdown(s3Exporter.shutdown),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithTimeout(config.TimeoutSettings),
	)
}

func createLogsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	cfg component.Config,
) (exporter.Logs, error) {
	config := cfg.(*Config)

	s3Exporter, err := newEnhanceIndexingS3Exporter(config, set.Logger)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewLogsExporter(
		ctx,
		set,
		cfg,
		s3Exporter.consumeLogs,
		exporterhelper.WithStart(s3Exporter.start),
		exporterhelper.WithShutdown(s3Exporter.shutdown),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithTimeout(config.TimeoutSettings),
	)
}
