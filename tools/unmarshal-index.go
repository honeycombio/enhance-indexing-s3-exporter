package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/honeycombio/enhance-indexing-s3-exporter/index"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <index-file>\n", os.Args[0])
		fmt.Println("  index-file: Path to a protobuf index file (e.g., index_trace_id_uuid.binpb or index_trace_id_uuid.binpb.gz)")
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
	fieldIndex := &index.FieldIndex{}
	err = fieldIndex.Unmarshal(data)
	if err != nil {
		fmt.Printf("Error unmarshaling protobuf: %v\n", err)
		os.Exit(1)
	}

	// Pretty print the result as JSON
	jsonData, err := json.MarshalIndent(fieldIndex, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling to JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Field Index Contents:\n")
	fmt.Printf("Field Name: %s\n", fieldIndex.FieldName)
	fmt.Printf("Index Data:\n%s\n", string(jsonData))

	// Print summary statistics
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Field Name: %s\n", fieldIndex.FieldName)
	fmt.Printf("  Unique Values: %d\n", len(fieldIndex.FieldIndex))

	totalFiles := 0
	for _, s3Keys := range fieldIndex.FieldIndex {
		totalFiles += len(s3Keys.S3Keys)
	}
	fmt.Printf("  Total S3 Files: %d\n", totalFiles)
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
