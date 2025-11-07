package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/collector/pdata/ptrace"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <traces-file>\n", os.Args[0])
		fmt.Println("  traces-file: Path to a protobuf OpenTelemetry traces file (e.g., traces.binpb or traces.binpb.gz)")
		os.Exit(1)
	}

	filePath := os.Args[1]

	// Read the file
	data, err := readFile(filePath)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Unmarshal the protobuf data
	unmarshaler := ptrace.ProtoUnmarshaler{}
	traces, err := unmarshaler.UnmarshalTraces(data)
	if err != nil {
		fmt.Printf("Error unmarshaling protobuf: %v\n", err)
		os.Exit(1)
	}

	// Convert to JSON for pretty printing
	marshaler := ptrace.JSONMarshaler{}
	jsonData, err := marshaler.MarshalTraces(traces)
	if err != nil {
		fmt.Printf("Error marshaling to JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OpenTelemetry Traces Contents:\n")

	// Pretty print the JSON
	var prettyJSON map[string]interface{}
	if err := json.Unmarshal(jsonData, &prettyJSON); err != nil {
		fmt.Printf("Raw JSON:\n%s\n", string(jsonData))
	} else {
		prettyData, err := json.MarshalIndent(prettyJSON, "", "  ")
		if err != nil {
			fmt.Printf("Raw JSON:\n%s\n", string(jsonData))
		} else {
			fmt.Printf("%s\n", string(prettyData))
		}
	}

	// Print summary statistics
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Resource spans: %d\n", traces.ResourceSpans().Len())

	totalSpans := 0
	uniqueTraceIDs := make(map[string]bool)
	uniqueSpanIDs := make(map[string]bool)
	serviceNames := make(map[string]bool)

	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		rs := traces.ResourceSpans().At(i)

		// Extract service name from resource
		if serviceName, exists := rs.Resource().Attributes().Get("service.name"); exists {
			serviceNames[serviceName.Str()] = true
		}

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)

			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				totalSpans++

				uniqueTraceIDs[span.TraceID().String()] = true
				uniqueSpanIDs[span.SpanID().String()] = true
			}
		}
	}

	fmt.Printf("Total spans: %d\n", totalSpans)
	fmt.Printf("Unique trace IDs: %d\n", len(uniqueTraceIDs))
	fmt.Printf("Unique span IDs: %d\n", len(uniqueSpanIDs))
	fmt.Printf("Services: %d\n", len(serviceNames))

	if len(serviceNames) > 0 {
		fmt.Printf("Service names:\n")
		for serviceName := range serviceNames {
			fmt.Printf("  - %s\n", serviceName)
		}
	}
}

// readFile reads a file, handling gzip decompression if needed
func readFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file

	// Check if file is gzipped by extension
	if strings.HasSuffix(strings.ToLower(filepath.Ext(filePath)), ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file contents: %w", err)
	}

	return data, nil
}
