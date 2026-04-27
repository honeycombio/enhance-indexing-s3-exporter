package enhanceindexings3exporter

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// noopS3Writer satisfies S3WriterInterface without touching a network.
type noopS3Writer struct{}

func (noopS3Writer) WriteBuffer(ctx context.Context, buf []byte, signalType string) (string, int, error) {
	return "", 0, nil
}

func (noopS3Writer) WriteBufferWithIndex(ctx context.Context, buf []byte, signalType, indexKey string) (string, int, error) {
	return "", 0, nil
}

// flakyS3Writer fails WriteBufferWithIndex for the first failUntil calls,
// then succeeds. Used to exercise the rolloverIndexes error/retry path.
type flakyS3Writer struct {
	failUntil int32
	calls     atomic.Int32
}

func (w *flakyS3Writer) WriteBuffer(ctx context.Context, buf []byte, signalType string) (string, int, error) {
	return w.WriteBufferWithIndex(ctx, buf, signalType, "")
}

func (w *flakyS3Writer) WriteBufferWithIndex(ctx context.Context, buf []byte, signalType, indexKey string) (string, int, error) {
	n := w.calls.Add(1)
	if n <= w.failUntil {
		return "", 0, fmt.Errorf("simulated transient S3 error (call %d)", n)
	}
	return "ok-key", 0, nil
}

// TestRolloverIndexes_ConcurrentWithAddTraces is a regression test for a data
// race between IndexManager.rolloverIndexes and IndexManager.addTracesToIndex
// (also addLogsToIndex and ensureMinuteBatch) on im.minuteIndexBatches.
//
// rolloverIndexes iterates and mutates the map without holding im.mutex, while
// the trace/log ingestion path writes to the map under im.mutex.Lock(). When
// a 30s tick coincides with incoming traffic, Go's runtime fatals the
// collector process with "concurrent map iteration and map write" or
// "concurrent map writes" (both uncatchable).
//
// Run with `go test -race` to observe the race cleanly; without -race, Go's
// map bookkeeping may fatal the process instead.
func TestRolloverIndexes_ConcurrentWithAddTraces(t *testing.T) {
	logger := zap.NewNop()
	config := &Config{
		IndexedFields: []fieldName{"user.id"},
		MarshalerName: awss3exporter.OtlpJSON,
		S3Uploader: awss3exporter.S3UploaderConfig{
			Compression: "gzip",
		},
	}

	im := NewIndexManager(config, logger)
	im.s3Writer = noopS3Writer{}

	// Pre-populate many non-current minute batches so rolloverIndexes has
	// plenty to iterate (and thus a longer race window).
	now := time.Now().UTC().Minute()
	for m := 0; m < 60; m++ {
		if m == now {
			continue
		}
		im.minuteIndexBatches[m] = &MinuteIndexBatch{
			fieldIndexes: make(map[fieldName]map[fieldValue]fieldS3Keys),
		}
	}

	ctx := context.Background()
	traces := createTestTraces()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writers: hammer addTracesToIndex on the current minute.
	writers := 4
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					im.addTracesToIndex(traces, "s3key-current", now)
				}
			}
		}()
	}

	// Reader: repeatedly roll over. In buggy code, this iterates the map
	// without holding the lock, racing the writers.
	for i := 0; i < 200; i++ {
		im.rolloverIndexes(ctx)
	}

	close(stop)
	wg.Wait()

	// The real assertion is "-race did not fire" and "the process did not
	// fatal with concurrent map access". If we reach this line, we're good.
	require.NotNil(t, im)
}

// TestRolloverIndexes_RetainsBatchOnUploadFailure is a regression test for the
// error-path behavior. On a transient upload failure, the stale minute batch
// must remain in im.minuteIndexBatches so the next tick retries it, rather
// than being silently dropped.
func TestRolloverIndexes_RetainsBatchOnUploadFailure(t *testing.T) {
	logger := zap.NewNop()
	config := &Config{
		IndexedFields: []fieldName{"user.id"},
		MarshalerName: awss3exporter.OtlpJSON,
		S3Uploader: awss3exporter.S3UploaderConfig{
			Compression: "gzip",
		},
	}

	im := NewIndexManager(config, logger)
	writer := &flakyS3Writer{failUntil: 1}
	im.s3Writer = writer

	// Install a ready-to-upload batch with real content so uploadBatch
	// actually calls the writer (empty batches are a no-op early-return).
	oldMinute := (time.Now().UTC().Minute() + 1) % 60
	im.minuteIndexBatches[oldMinute] = &MinuteIndexBatch{
		minuteDir: "traces-and-logs/year=2026/month=04/day=22/hour=00/minute=XX",
		fieldIndexes: map[fieldName]map[fieldValue]fieldS3Keys{
			"user.id": {"u1": {"s3key1": struct{}{}}},
		},
	}

	ctx := context.Background()

	// First tick: upload fails, batch must be retained for retry.
	im.rolloverIndexes(ctx)
	require.Contains(t, im.minuteIndexBatches, oldMinute,
		"batch must be retained in the map after a transient upload failure")
	assert.Equal(t, int32(1), writer.calls.Load())

	// Second tick: upload succeeds, batch is cleaned up.
	im.rolloverIndexes(ctx)
	assert.NotContains(t, im.minuteIndexBatches, oldMinute,
		"batch must be removed from the map after a successful upload")
}
