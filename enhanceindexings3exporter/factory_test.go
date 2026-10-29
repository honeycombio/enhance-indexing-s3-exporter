package enhanceindexings3exporter

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestGetOrCreateIndexManagerConcurrency(t *testing.T) {
	// Reset the global state for testing
	indexManagers = nil
	indexManagersOnce = sync.Once{}

	config := &Config{}
	logger := zap.NewNop()

	// Test concurrent access to getOrCreateIndexManager
	const numGoroutines = 100
	const numCallsPerGoroutine = 10

	var wg sync.WaitGroup
	results := make(chan *IndexManager, numGoroutines*numCallsPerGoroutine)

	// Create multiple goroutines that call getOrCreateIndexManager concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numCallsPerGoroutine; j++ {
				componentID := component.MustNewID("test")
				manager := getOrCreateIndexManager(componentID, config, logger)
				results <- manager
			}
		}(i)
	}

	wg.Wait()
	close(results)

	// Verify all returned managers are the same instance (singleton behavior)
	var firstManager *IndexManager
	count := 0
	for manager := range results {
		if firstManager == nil {
			firstManager = manager
		}
		if manager != firstManager {
			t.Errorf("Expected all managers to be the same instance, got different instances")
		}
		count++
	}

	if count != numGoroutines*numCallsPerGoroutine {
		t.Errorf("Expected %d results, got %d", numGoroutines*numCallsPerGoroutine, count)
	}

	// Verify the map was initialized
	if indexManagers == nil {
		t.Error("Expected indexManagers map to be initialized")
	}
}

func TestGetOrCreateIndexManagerDifferentIDs(t *testing.T) {
	// Reset the global state for testing
	indexManagers = nil
	indexManagersOnce = sync.Once{}

	config := &Config{}
	logger := zap.NewNop()

	// Test that different component IDs get different managers
	id1 := component.MustNewID("test1")
	id2 := component.MustNewID("test2")

	manager1 := getOrCreateIndexManager(id1, config, logger)
	manager2 := getOrCreateIndexManager(id2, config, logger)

	if manager1 == manager2 {
		t.Error("Expected different component IDs to get different managers")
	}

	// Test that same ID gets same manager
	manager1Again := getOrCreateIndexManager(id1, config, logger)
	if manager1 != manager1Again {
		t.Error("Expected same component ID to get same manager instance")
	}
}

func TestIndexManagersInitializationRace(t *testing.T) {
	// Reset the global state for testing
	indexManagers = nil
	indexManagersOnce = sync.Once{}

	config := &Config{}
	logger := zap.NewNop()

	// Test that multiple goroutines can safely initialize the map
	const numGoroutines = 50
	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errors <- fmt.Errorf("panic during initialization: %v", r)
				}
			}()

			componentID := component.MustNewID("test")
			_ = getOrCreateIndexManager(componentID, config, logger)
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Error during concurrent initialization: %v", err)
	}

	// Verify the map was initialized
	if indexManagers == nil {
		t.Error("Expected indexManagers map to be initialized")
	}
}

func TestIndexManagerRestartsAfterConfigReload(t *testing.T) {
	indexManagers = nil
	indexManagersOnce = sync.Once{}

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx := context.Background()
	id := component.MustNewID("reload_test")

	// The traces and logs exporters of one pipeline share a manager.
	tracesManager := getOrCreateIndexManager(id, &Config{}, logger)
	logsManager := getOrCreateIndexManager(id, &Config{}, logger)
	require.Same(t, tracesManager, logsManager)
	require.NoError(t, tracesManager.start(ctx, noopS3Writer{}))
	require.NoError(t, logsManager.start(ctx, noopS3Writer{}))

	// Shutting down one signal must not stop the timer for the other.
	require.NoError(t, tracesManager.shutdown(ctx))
	assert.False(t, logsManager.isStopped())
	require.NoError(t, logsManager.shutdown(ctx))
	assert.True(t, tracesManager.isStopped())

	// An in-place config reload rebuilds the exporters with the same component ID.
	reloadedConfig := &Config{}
	reloadedManager := getOrCreateIndexManager(id, reloadedConfig, logger)
	require.NotSame(t, tracesManager, reloadedManager)
	require.NoError(t, reloadedManager.start(ctx, noopS3Writer{}))
	t.Cleanup(func() { _ = reloadedManager.shutdown(ctx) })

	assert.Equal(t, 2, logs.FilterMessage("Starting index batch timer").Len())
	assert.Same(t, reloadedConfig, reloadedManager.config)
}
