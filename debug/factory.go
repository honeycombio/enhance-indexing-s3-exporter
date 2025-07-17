package debug

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
)

const (
	typeStr   = "debug"
	stability = component.StabilityLevelAlpha
)

// NewFactory creates a new debug exporter factory
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		typeStr,
		CreateDefaultConfig,
		exporter.WithTraces(createTracesExporter, stability),
		exporter.WithLogs(createLogsExporter, stability),
	)
}

// createTracesExporter creates a traces exporter
func createTracesExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	cfg component.Config,
) (exporter.Traces, error) {
	config := cfg.(*Config)

	debugExporter := newDebugExporter(config, set.Logger)

	return exporterhelper.NewTracesExporter(
		ctx,
		set,
		cfg,
		debugExporter.consumeTraces,
		exporterhelper.WithStart(debugExporter.start),
		exporterhelper.WithShutdown(debugExporter.shutdown),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithTimeout(config.TimeoutSettings),
	)
}

// createLogsExporter creates a logs exporter
func createLogsExporter(
	ctx context.Context,
	set exporter.CreateSettings,
	cfg component.Config,
) (exporter.Logs, error) {
	config := cfg.(*Config)

	debugExporter := newDebugExporter(config, set.Logger)

	return exporterhelper.NewLogsExporter(
		ctx,
		set,
		cfg,
		debugExporter.consumeLogs,
		exporterhelper.WithStart(debugExporter.start),
		exporterhelper.WithShutdown(debugExporter.shutdown),
		exporterhelper.WithQueue(config.QueueSettings),
		exporterhelper.WithTimeout(config.TimeoutSettings),
	)
}
