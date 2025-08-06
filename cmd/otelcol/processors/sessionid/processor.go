package sessionid

import (
	"context"
	"math/rand"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.uber.org/zap"
)

var typeStr = component.MustNewType("sessionid")

type config struct{}

type sessionIdProcessor struct {
	config config
	logger *zap.Logger
}

// NewFactory creates a new processor factory.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithTraces(createExampleProcessor, component.StabilityLevelAlpha),
	)
}

// createDefaultConfig returns the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &config{}
}

// createExampleProcessor initializes an instance of the example processor.
func createExampleProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	// Convert baseCfg to the correct type.
	config := cfg.(*config)

	// Create a new processor instance.
	p := newSessionIdProcessor(config, set.Logger)

	// Wrap the processor with the helper utilities.
	return processorhelper.NewTraces(
		ctx,
		set,
		config,
		next,
		p.consumeTraces,
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
}

// newSessionIdProcessor constructs a new instance of the example processor.
func newSessionIdProcessor(cfg *config, logger *zap.Logger) *sessionIdProcessor {
	return &sessionIdProcessor{
		config: *cfg,
		logger: logger,
	}
}

// consumeTraces modify traces adding session.id to each span.
func (p *sessionIdProcessor) consumeTraces(_ context.Context, traces ptrace.Traces) (ptrace.Traces, error) {
	sessionId := int64(rand.Intn(10))

	p.logger.Info("adding session.id to traces", zap.Int64("session_id", sessionId))
	for _, rs := range traces.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				span.Attributes().PutInt("session.id", sessionId)
			}
		}
	}

	return traces, nil
}
