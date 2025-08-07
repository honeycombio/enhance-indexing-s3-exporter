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

type sessionIdProcessor struct {
	logger *zap.Logger
}

// NewFactory creates a new processor factory.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithTraces(
			createSessionIdProcessor,
			// In the future we may want to change this to be Beta or even Stable.
			component.StabilityLevelAlpha,
		),
	)
}

// createDefaultConfig returns the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &struct{}{}
}

// createSessionIdProcessor initializes an instance of the sessionid processor.
func createSessionIdProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	p := &sessionIdProcessor{
		logger: set.Logger,
	}

	return processorhelper.NewTraces(
		ctx,
		set,
		cfg,
		next,
		p.consumeTraces,
		processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}),
	)
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
