package metrics

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConfig implements the Config interface for testing
type mockConfig struct {
	MarshalerName awss3exporter.MarshalerType
	APIEndpoint   string
}

func (c *mockConfig) GetMarshalerName() awss3exporter.MarshalerType {
	return c.MarshalerName
}

func (c *mockConfig) GetAPIEndpoint() string {
	return c.APIEndpoint
}

func TestNewExporterMetrics(t *testing.T) {
	config := &mockConfig{
		MarshalerName: awss3exporter.OtlpJSON,
		APIEndpoint:   "http://localhost:8086",
	}

	metrics, err := NewExporterMetrics(config)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify all metric instruments were created
	assert.NotNil(t, metrics.meter)
	assert.NotNil(t, metrics.spanCountTotal)
	assert.NotNil(t, metrics.spanBytesTotal)
	assert.NotNil(t, metrics.logCountTotal)
	assert.NotNil(t, metrics.logBytesTotal)

	// Verify attributes were pre-computed
	assert.NotNil(t, metrics.attrs)
	assert.Len(t, metrics.attrs, 2) // marshaler + api_endpoint
}

func TestNewExporterMetricsWithoutAPIEndpoint(t *testing.T) {
	config := &mockConfig{
		MarshalerName: awss3exporter.OtlpProto,
		APIEndpoint:   "",
	}

	metrics, err := NewExporterMetrics(config)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// Verify attributes - should only have marshaler, not api_endpoint
	assert.NotNil(t, metrics.attrs)
	assert.Len(t, metrics.attrs, 1) // only marshaler
}

func TestExporterMetricsThreadSafety(t *testing.T) {
	// Test that metrics creation and concurrent recording works without panics
	config := &mockConfig{
		MarshalerName: awss3exporter.OtlpJSON,
		APIEndpoint:   "http://localhost:8086",
	}
	metrics, err := NewExporterMetrics(config)
	require.NoError(t, err)

	ctx := context.Background()

	// Run concurrent metric updates
	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 100; i++ {
			metrics.AddSpanMetrics(ctx, 1, 100)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			metrics.AddLogMetrics(ctx, 1, 200)
		}
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify metrics instance is still valid after concurrent operations
	assert.NotNil(t, metrics.meter)
	assert.NotNil(t, metrics.spanCountTotal)
	assert.NotNil(t, metrics.spanBytesTotal)
	assert.NotNil(t, metrics.logCountTotal)
	assert.NotNil(t, metrics.logBytesTotal)
}

func TestAddSpanMetrics(t *testing.T) {
	config := &mockConfig{
		MarshalerName: awss3exporter.OtlpJSON,
		APIEndpoint:   "http://localhost:8086",
	}
	metrics, err := NewExporterMetrics(config)
	require.NoError(t, err)

	ctx := context.Background()

	// These calls should not panic
	metrics.AddSpanMetrics(ctx, 10, 1024)
	metrics.AddSpanMetrics(ctx, 5, 512)

	// Note: We can't verify the actual metric values as OpenTelemetry counters are write-only
}

func TestAddLogMetrics(t *testing.T) {
	config := &mockConfig{
		MarshalerName: awss3exporter.OtlpJSON,
		APIEndpoint:   "http://localhost:8086",
	}
	metrics, err := NewExporterMetrics(config)
	require.NoError(t, err)

	ctx := context.Background()

	// These calls should not panic
	metrics.AddLogMetrics(ctx, 20, 2048)
	metrics.AddLogMetrics(ctx, 15, 1536)

	// Note: We can't verify the actual metric values as OpenTelemetry counters are write-only
}
