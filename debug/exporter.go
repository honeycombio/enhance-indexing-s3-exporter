package debug

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

// debugExporter implements the debug exporter
type debugExporter struct {
	logger    *zap.Logger
	config    *Config
	verbosity configtelemetry.Level
}

// newDebugExporter creates a new debug exporter
func newDebugExporter(config *Config, logger *zap.Logger) *debugExporter {
	return &debugExporter{
		logger:    logger,
		config:    config,
		verbosity: config.Verbosity,
	}
}

// start is called when the exporter starts
func (e *debugExporter) start(ctx context.Context, host component.Host) error {
	var verbosityStr string
	switch e.verbosity {
	case configtelemetry.LevelBasic:
		verbosityStr = "basic"
	case configtelemetry.LevelNormal:
		verbosityStr = "normal"
	case configtelemetry.LevelDetailed:
		verbosityStr = "detailed"
	default:
		verbosityStr = "unknown"
	}
	e.logger.Info("Debug exporter started", zap.String("verbosity", verbosityStr))
	return nil
}

// shutdown is called when the exporter shuts down
func (e *debugExporter) shutdown(ctx context.Context) error {
	e.logger.Info("Debug exporter stopped")
	return nil
}

// consumeTraces implements the traces consumer interface
func (e *debugExporter) consumeTraces(ctx context.Context, traces ptrace.Traces) error {
	if e.verbosity == configtelemetry.LevelBasic {
		fmt.Printf("DEBUG EXPORTER: Received %d traces\n", traces.ResourceSpans().Len())
		return nil
	}

	fmt.Println("=== DEBUG EXPORTER: TRACES ===")
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)
		fmt.Printf("Resource Span %d:\n", i+1)

		if e.verbosity == configtelemetry.LevelDetailed {
			fmt.Printf("  Resource Attributes: %s\n", formatAttributes(rs.Resource().Attributes()))
		}

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			fmt.Printf("  Scope %d: %s (version: %s)\n", j+1, ss.Scope().Name(), ss.Scope().Version())

			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				fmt.Printf("    Span %d: %s (trace_id: %s, span_id: %s)\n",
					k+1, span.Name(), span.TraceID().String(), span.SpanID().String())

				if e.verbosity == configtelemetry.LevelDetailed {
					fmt.Printf("      Attributes: %s\n", formatAttributes(span.Attributes()))
					if span.Events().Len() > 0 {
						fmt.Printf("      Events: %d\n", span.Events().Len())
					}
					if span.Links().Len() > 0 {
						fmt.Printf("      Links: %d\n", span.Links().Len())
					}
				}
			}
		}
	}
	fmt.Println("=== END TRACES ===")
	return nil
}

// consumeLogs implements the logs consumer interface
func (e *debugExporter) consumeLogs(ctx context.Context, logs plog.Logs) error {
	if e.verbosity == configtelemetry.LevelBasic {
		fmt.Printf("DEBUG EXPORTER: Received %d logs\n", logs.ResourceLogs().Len())
		return nil
	}

	fmt.Println("=== DEBUG EXPORTER: LOGS ===")
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		fmt.Printf("Resource Logs %d:\n", i+1)

		if e.verbosity == configtelemetry.LevelDetailed {
			fmt.Printf("  Resource Attributes: %s\n", formatAttributes(rl.Resource().Attributes()))
		}

		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			fmt.Printf("  Scope %d: %s (version: %s)\n", j+1, sl.Scope().Name(), sl.Scope().Version())

			for k := 0; k < sl.LogRecords().Len(); k++ {
				log := sl.LogRecords().At(k)
				fmt.Printf("    Log %d: %s (level: %s, timestamp: %s)\n",
					k+1, log.Body().AsString(), log.SeverityText(), log.Timestamp().String())

				if e.verbosity == configtelemetry.LevelDetailed {
					fmt.Printf("      Attributes: %s\n", formatAttributes(log.Attributes()))
					if !log.TraceID().IsEmpty() {
						fmt.Printf("      Trace ID: %s\n", log.TraceID().String())
					}
					if !log.SpanID().IsEmpty() {
						fmt.Printf("      Span ID: %s\n", log.SpanID().String())
					}
				}
			}
		}
	}
	fmt.Println("=== END LOGS ===")
	return nil
}

// formatAttributes formats attributes for display
func formatAttributes(attrs pcommon.Map) string {
	if attrs.Len() == 0 {
		return "{}"
	}

	var pairs []string
	attrs.Range(func(k string, v pcommon.Value) bool {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, v.AsString()))
		return true
	})

	return "{" + strings.Join(pairs, ", ") + "}"
}
