package enhanceindexings3exporter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awss3exporter"
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
