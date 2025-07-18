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

var componentType = component.MustNewType(typeStr)

// NewFactory creates a new debug exporter factory
func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		componentType,
		CreateDefaultConfig,
		exporter.WithTraces(createTracesExporter, stability),
		exporter.WithLogs(createLogsExporter, stability),
	)
}

// createTracesExporter creates a traces exporter
func createTracesExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Traces, error) {
	config := cfg.(*Config)

	debugExporter := newDebugExporter(config, set.Logger)

	return exporterhelper.NewTraces(
		ctx,
		set,
		cfg,
		debugExporter.consumeTraces,
		exporterhelper.WithStart(debugExporter.start),
		exporterhelper.WithShutdown(debugExporter.shutdown),
	)
}

// createLogsExporter creates a logs exporter
func createLogsExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Logs, error) {
	config := cfg.(*Config)

	debugExporter := newDebugExporter(config, set.Logger)

	return exporterhelper.NewLogs(
		ctx,
		set,
		cfg,
		debugExporter.consumeLogs,
		exporterhelper.WithStart(debugExporter.start),
		exporterhelper.WithShutdown(debugExporter.shutdown),
	)
}
