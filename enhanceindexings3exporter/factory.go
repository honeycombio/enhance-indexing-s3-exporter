package enhanceindexings3exporter

import (
	"context"
	"sync"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.uber.org/zap"
)

const (
	typeStr   = "enhance_indexing_s3_exporter"
	stability = component.StabilityLevelAlpha
)

var (
	componentType     = component.MustNewType(typeStr)
	indexManagers     map[component.ID]*IndexManager
	indexManagerMutex sync.RWMutex
	indexManagersOnce sync.Once
)

// getOrCreateIndexManager returns an existing IndexManager for the component ID or creates a new one
func getOrCreateIndexManager(id component.ID, config *Config, logger *zap.Logger) *IndexManager {
	// Ensure indexManagers map is initialized exactly once
	indexManagersOnce.Do(func() {
		indexManagers = make(map[component.ID]*IndexManager)
	})

	indexManagerMutex.Lock()
	defer indexManagerMutex.Unlock()

	if indexManager, exists := indexManagers[id]; exists {
		return indexManager
	}

	indexManager := NewIndexManager(config, logger)
	indexManagers[id] = indexManager
	return indexManager
}

func NewFactory() exporter.Factory {
	return exporter.NewFactory(
		componentType,
		createDefaultConfig,
		exporter.WithTraces(createTracesExporter, stability),
		exporter.WithLogs(createLogsExporter, stability),
	)
}

func createTracesExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Traces, error) {
	config := cfg.(*Config)

	set.Logger.Info("Creating traces exporter", zap.String("componentID", set.ID.String()))

	var indexManager *IndexManager
	if config.IndexConfig.Enabled {
		indexManager = getOrCreateIndexManager(set.ID, config, set.Logger)
	}

	s3Exporter, err := newEnhanceIndexingS3Exporter(config, set.Logger, indexManager)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewTraces(
		ctx,
		set,
		cfg,
		s3Exporter.consumeTraces,
		exporterhelper.WithStart(s3Exporter.start),
		exporterhelper.WithShutdown(s3Exporter.shutdown),
		exporterhelper.WithQueueBatch(config.QueueBatchConfig, exporterhelper.NewTracesQueueBatchSettings()),
		exporterhelper.WithTimeout(config.TimeoutConfig),
		exporterhelper.WithRetry(config.RetryConfig),
	)
}

func createLogsExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Logs, error) {
	config := cfg.(*Config)

	set.Logger.Info("Creating logs exporter", zap.String("componentID", set.ID.String()))

	var indexManager *IndexManager
	if config.IndexConfig.Enabled {
		indexManager = getOrCreateIndexManager(set.ID, config, set.Logger)
	}

	s3Exporter, err := newEnhanceIndexingS3Exporter(config, set.Logger, indexManager)
	if err != nil {
		return nil, err
	}

	return exporterhelper.NewLogs(
		ctx,
		set,
		cfg,
		s3Exporter.consumeLogs,
		exporterhelper.WithStart(s3Exporter.start),
		exporterhelper.WithShutdown(s3Exporter.shutdown),
		exporterhelper.WithQueueBatch(config.QueueBatchConfig, exporterhelper.NewLogsQueueBatchSettings()),
		exporterhelper.WithTimeout(config.TimeoutConfig),
		exporterhelper.WithRetry(config.RetryConfig),
	)
}
