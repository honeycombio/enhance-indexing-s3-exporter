# Enhance Indexing S3 Exporter

This exporter extends the OpenTelemetry AWS S3 exporter with both automatic and custom field indexing capabilities. It writes telemetry data to S3 while simultaneously generating field-based indexes for more efficient event rehydration and querying in Honeycomb.

## Features

- **Compatible Configuration**: Configuration options fully compatible with publicly available [awss3exporter](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/exporter/awss3exporter) component
- **S3 Storage**: Exports traces, logs, and indexes to S3 in OTLP protobuf or JSON format
- **Automatic Field Indexing**: Generates indexes for trace IDs, service names, and session IDs
- **Custom Field Indexing**: Flexibility to include additional fields for indexing
- **Compression**: Optional gzip compression for data and index files

## Quick Start

### Prerequisites

- Go 1.24 or later
- Docker (for local development)
- AWS credentials configured (to interact with AWS S3 buckets)
- Honeycomb Management API key and secret with `enhance:write` scope
  - See [Managing API Keys](https://docs.honeycomb.io/configure/teams/manage-api-keys/) for details on creating Management API keys

### Optional Support

- [Tilt](https://docs.tilt.dev) for local dev orchestration of Docker infrastructure

### Installation

Build the OpenTelemetry Collector with the enhance-indexing-s3-exporter:

```bash
cd cmd/otelcol
go build -o otelcol
```

### Configuration

Create a collector configuration file with the exporter. Here's a minimal example:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

exporters:
  enhance_indexing_s3_exporter:
    # Required: Honeycomb API credentials
    api_key: ${env:HONEYCOMB_MANAGEMENT_API_KEY}
    api_secret: ${env:HONEYCOMB_MANAGEMENT_API_SECRET}
    api_endpoint: https://api.honeycomb.io

    # Required: S3 configuration
    s3uploader:
      region: "us-west-2"
      s3_bucket: "telemetry-data"
      s3_partition_format: "year=%Y/month=%m/day=%d/hour=%H/minute=%M"
      compression: "gzip"

    # Required: Data format
    marshaler: "otlp_protobuf"

    # Optional: Custom indexed fields
    indexed_fields: ["user.id", "customer.id"]

service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [enhance_indexing_s3_exporter]
    logs:
      receivers: [otlp]
      exporters: [enhance_indexing_s3_exporter]
```

For complete configuration options, see the [Configuration Guide](enhanceindexings3exporter/README.md).

### Running

```bash
./otelcol --config config.yaml
```

## Local Development with Tilt

### 1. Start Docker Daemon

Ensure the Docker daemon is running on your system.

### 2. Start the Project with Tilt

Use Tilt to start and manage the development environment:

```bash
tilt up
```

### 3. Send Test Signals

Open your browser and navigate to `http://localhost:10350`. Use one of the available generators to send logs or traces:

- `otelgen_one_log`
- `otelgen_one_trace`
- `otelgen_logs_stream`
- `otelgen_traces_stream`

### 4. Examine Locally Uploaded Files

View the locally uploaded files via the Minio S3 bucket at `http://localhost:9001`.

Log in with the following credentials:
- Username: `minioadmin`
- Password: `minioadmin`

Browse the buckets to verify the uploaded data and index files.

## Documentation

- **[Configuration Guide](enhanceindexings3exporter/README.md)** - Complete configuration options, validation rules, and examples
- **[Tools README](tools/README.md)** - Utility tools for working with index files
- **[CHANGELOG](CHANGELOG.md)** - Version history and release notes
- **[CONTRIBUTING](CONTRIBUTING.md)** - Contribution guidelines
- **[RELEASING](RELEASING.md)** - Release process

## How It Works

1. The OpenTelemetry Collector receives telemetry data via the OTLP receiver
2. The enhance-indexing-s3-exporter processes traces and logs
3. Trace and log data is exported to S3 in time-partitioned directories with OTLP protobuf or JSON format
4. Field indexes are automatically generated, mapping trace and log field values to S3 file locations
5. Index files are uploaded alongside data files in the same time partition

### Indexed Fields

The exporter automatically indexes three fields:
- `trace.trace_id` - Trace identifier from spans and logs
- `service.name` - Service name from resource/scope/span attributes
- `session.id` - Session identifier from resource/scope/span attributes

You can configure additional custom fields to index. See the [Configuration Guide](enhanceindexings3exporter/README.md#custom-indexed-fields) for details.

### Exported File Structure

Data and index files are organized in S3 using time-based partitioning and use the following naming conventions for protobuf-encoded, gzipped files:

```
{bucket}/{partition}/traces_{uuid}.binpb.gz
{bucket}/{partition}/logs_{uuid}.binpb.gz
{bucket}/{partition}/index_trace.trace_id_{uuid}.binpb.gz
{bucket}/{partition}/index_service.name_{uuid}.binpb.gz
{bucket}/{partition}/index_{custom_field}_{uuid}.binpb.gz
```

For detailed output format specifications, see the [Configuration Guide](enhanceindexings3exporter/README.md#output-structure).

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.

## Support

- Report issues at https://github.com/honeycombio/enhance-indexing-s3-exporter/issues
- See our [Code of Conduct](CODE_OF_CONDUCT.md)
