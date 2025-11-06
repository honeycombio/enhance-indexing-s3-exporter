# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

### The Golden Rule

When unsure about implementation details, ALWAYS ask the developer.
Do not make assumptions about business logic or system behavior.

### What AI Must Do

1. **Only modify tests when given permission** - Tests encode human intent, so unless directed to add/edit tests, you should ask before modifying them.
2. **Never commit secrets** - Use environment variables. Never run `git add .` or add all files to a commit—always add specific files you edited.
3. **Never assume business logic** - Always ask

## Development Commands

### Building and Testing
- `go build -o /dev/null ./cmd/otelcol` - Build the OpenTelemetry collector with enhance-indexing-s3-exporter
- `go build -o /dev/null ./enhanceindexings3exporter` - Build the exporter package
- `go test -v ./...` - Run all tests
- `go test -v -run TestSomeName ./enhanceindexings3exporter` - Run specific test

### Local Development Environment
- Use Tilt for local development: `tilt up`
- Access test generators at http://localhost:10350
- Access MinIO console at http://localhost:9001 (username: minioadmin, password: minioadmin)

### Code Generation
- `./tools/build_proto.sh` - Generate protobuf code for index files

### Go Code Style Guidelines
- gofmt with tabs (not spaces), imports ordered (std lib, external, internal)
- Prefer `fmt.Errorf()` over `errors.New(fmt.Sprintf())`
- No `spew.Dump()` in production code

### Rules for Go Tests
- Test naming: `Test<test name>`
- Use `github.com/stretchr/testify/assert` and `github.com/stretchr/testify/require` for test assertions
- DO NOT add t.Skip() or t.Logf() calls except during active debugging
- Write tests in a separate `packagename_test` package when possible to test the external API

## Architecture

### Core Components
- **enhanceindexings3exporter** - OpenTelemetry exporter that writes telemetry data to S3 with field indexing
- **index** - Index data structures and protobuf definitions for field indexes
- **cmd/otelcol** - OpenTelemetry Collector distribution with the enhance-indexing-s3-exporter
- **tools** - Utility tools including unmarshal-index for inspecting index files

### Key Data Flow
1. OpenTelemetry Collector receives telemetry data via OTLP receiver
2. Enhance-indexing-s3-exporter processes traces and logs
3. Data is written to S3 in OTLP protobuf or JSON format
4. Field indexes are generated and uploaded alongside data files
5. Indexes map field values to S3 file locations for efficient querying

### Configuration
- Exporter configuration is defined in `enhanceindexings3exporter/config.go`
- See `enhanceindexings3exporter/README.md` for full configuration documentation
- Example configs available in the Tilt development environment

## General Rules
- Never add new values to the context.Context
- Avoid creating test-only exceptions in the code
- Never use Go commands with the -i flag
- When explaining what you're doing, be concise and stick to just the facts. Avoid exclamation points.
- Do not add unnecessary comments to the code. Only comment extremely tricky or difficult to understand code.
