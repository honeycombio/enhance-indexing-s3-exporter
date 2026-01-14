# Enhance Indexing S3 Exporter Configuration

This exporter extends the OpenTelemetry AWS S3 exporter with both automatic and custom field indexing capabilities. It writes trace and log telemetry data to S3 while simultaneously building, then exporting field-based indexes for more efficient event rehydration and querying in Honeycomb.

## Supported Configuration Options

### Core Configuration Structure

```yaml
exporters:
   enhance_indexing_s3_exporter:
    # Standard OpenTelemetry exporter settings
    sending_queue: # Queue configuration
    timeout: # Timeout settings  
    retry_on_failure: # Retry configuration
   
    # Honeycomb API Key configuration
    api_key: ${env:HONEYCOMB_MANAGEMENT_API_KEY}
    api_secret: ${env:HONEYCOMB_MANAGEMENT_API_SECRET}
    api_endpoint: https://api.honeycomb.io  

    # S3 uploader configuration (required)
    s3uploader:
      # ... S3 settings
    
    # Data marshaling format (required) 
    marshaler: "otlp_protobuf"  # or "otlp_json"
   
    # Custom index fields (optional)
    indexed_fields: ["user.id", "customer.id"]
```

### Honeycomb API Configuration

The exporter requires a Honeycomb Management API key with the `enhance:write` scope for authentication and usage tracking.

For information on creating Management API keys, see [Managing API Keys](https://docs.honeycomb.io/configure/teams/manage-api-keys/).

| Field                      | Description                                                                     | Required | Default |
|----------------------------|---------------------------------------------------------------------------------|----------|---------|
| `api_key`                  | Management API key for your Honeycomb account (must have `enhance:write` scope) | Yes      | -       |
| `api_secret`               | Management API secret for your Honeycomb account                                | Yes      | -       |
| `api_endpoint`             | Honeycomb API endpoint URL for authentication and usage tracking                | Yes      | -       |
| `usage_reporting_interval` | How often usage metrics are sent to Honeycomb. Valid range: 30s to 10m. | No | 1m |


### S3 Uploader Configuration (`s3uploader`)

| Field                 | Description                                                                                             | Default                                     |
|-----------------------|---------------------------------------------------------------------------------------------------------|---------------------------------------------|
| `region`              | AWS region (required)                                                                                   | "us-east-1"                                 |
| `s3_bucket`           | S3 bucket (required unless `endpoint` provided)                                                         | -                                           |
| `s3_prefix`           | The prefix to use when writing files to S3.                                                             | -                                           |
| `s3_partition_format` | Filepath partitioning format (see [strftime][1]). Must contain year/month/day/hour/minute placeholders. | "year=%Y/month=%m/day=%d/hour=%H/minute=%M" |
| `compression`         | File compression: "gzip" or "none"                                                                      | "gzip"                                      |
| `retry_mode`          | Retry strategy: "standard" or "adaptive"                                                                | "standard"                                  |
| `retry_max_attempts`  | Maximum retry attempts                                                                                  | 3                                           |
| `endpoint`            | Custom S3 endpoint (for S3-compatible services)                                                         | -                                           |
| `s3_force_path_style` | Force path-style S3 addressing                                                                          | false                                       |
| `disable_ssl`         | Disable SSL for S3 requests                                                                             | false                                       |

[1]: https://www.man7.org/linux/man-pages/man3/strftime.3.html

**Note**: `file_prefix` is **not supported** and will cause validation to fail.

### Marshaler Configuration

The `marshaler` field determines the data format written to S3:

- **`otlp_protobuf`** (default): OpenTelemetry Protocol as Protocol Buffers - more efficient, smaller files
- **`otlp_json`**: OpenTelemetry Protocol as JSON - human-readable format

#### Automatically Indexed Fields

When using this component, these fields are **automatically indexed**:

- **`trace.trace_id`**: Trace identifier (from span/log trace ID)
- **`service.name`**: Service name (from resource/scope/item attributes)  
- **`session.id`**: Session identifier (from resource/scope/item attributes)

#### Custom Indexed Fields

The `indexed_fields` field can be configured to specify a list of additional fields to have indexed:

```yaml
exporters:
  enhance_indexing_s3_exporter:
  [...]
    indexed_fields: ["user.id", "customer.id", "environment", "version"]
```

**Note**: The `indexed_fields` configuration specifies which custom fields to index in addition to the automatically indexed fields (`trace.trace_id`, `service.name`, `session.id`). If `indexed_fields` is empty or not specified, only the automatic fields will be indexed.

#### Field Value Precedence

When the same field appears in multiple attribute locations, values are resolved with the following precedence (highest to lowest):

1. **Item attributes** (span attributes for traces, log record attributes for logs)
2. **Instrumentation scope attributes**
3. **Resource attributes**

### Configuration Validation

The exporter validates configuration with these rules:

- ✅ `region` is required
- ✅ `s3_bucket` is required unless `endpoint` is provided
- ✅ `compression` must be "gzip" or "none"
- ✅ `retry_mode` must be "standard" or "adaptive"
- ✅ `marshaler` must be "otlp_json" or "otlp_protobuf"
- ✅ `s3_partition_format` must contain year/month/day/hour/minute placeholders
- ✅ `s3_partition_format` cannot start or end with "/"
- ✅ `api_key` is required
- ✅ `api_secret` is required
- ✅ `api_endpoint` is required and must start with "http://" or "https://"
- ✅ `usage_reporting_interval` must be between 30s and 10m (standalone mode only)
- ❌ `file_prefix` is not supported (will cause validation failure)

## Example Configurations

### Basic Component Configuration

```yaml
exporters:
  enhance_indexing_s3_exporter:
    # Required Honeycomb API credentials
    api_key: ${env:HONEYCOMB_MANAGEMENT_API_KEY}
    api_secret: ${env:HONEYCOMB_MANAGEMENT_API_SECRET}
    api_endpoint: https://api.honeycomb.io

    # S3 configuration
    s3uploader:
      region: "us-west-2"
      s3_bucket: "telemetry-data"
      s3_partition_format: "year=%Y/month=%m/day=%d/hour=%H/minute=%M"
      compression: "gzip"

    # Data format
    marshaler: "otlp_protobuf"
```

### Full Configuration Example with Custom Index Fields and Queue Batching

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

exporters:
  enhance_indexing_s3_exporter:
    # Queue, timeout, and retry settings
    sending_queue:
        batch:
            flush_timeout: 30s
            max_size: 50000
            min_size: 50000
        enabled: true
        queue_size: 500000
        sizer: items
    timeout: 30s
    retry_on_failure:
      enabled: true
      initial_interval: 5s
      max_interval: 30s
   
    # Honeycomb API Management Key & Secret configuration
    api_key: ${env:HONEYCOMB_MANAGEMENT_API_KEY}
    api_secret: ${env:HONEYCOMB_MANAGEMENT_API_SECRET}
    api_endpoint: https://api.honeycomb.io  

    # S3 configuration
    s3uploader:
      region: "us-west-2"
      s3_bucket: "telemetry-data"
      s3_partition_format: "year=%Y/month=%m/day=%d/hour=%H/minute=%M"
      compression: "gzip"
      retry_mode: "adaptive"
      retry_max_attempts: 5
    
    # Data format
    marshaler: "otlp_protobuf"
    
    # Field indexing
    indexed_fields:
      - "user.id"
      - "customer.id"
      - "environment"
      - "deployment.version"
    # Can also one-line the index list
    # indexed_fields: ["user.id", "customer.id", "environment", "deployment.version"]

# Pipeline configuration
service:
  pipelines:
    traces:
      receivers: [otlp]
      exporters: [enhance_indexing_s3_exporter]
    logs:
      receivers: [otlp]
      exporters: [enhance_indexing_s3_exporter]
```

### Local Development with MinIO

```yaml
exporters:
  enhance_indexing_s3_exporter:
    # Required Honeycomb API credentials (even for local development)
    api_key: ${env:HONEYCOMB_MANAGEMENT_API_KEY}
    api_secret: ${env:HONEYCOMB_MANAGEMENT_API_SECRET}
    api_endpoint: https://api.honeycomb.io

    # MinIO configuration
    s3uploader:
      region: "us-east-1"
      endpoint: "http://localhost:9000"
      s3_bucket: "telemetry-bucket"
      s3_force_path_style: true
      disable_ssl: true
      s3_partition_format: "year=%Y/month=%m/day=%d/hour=%H/minute=%M"
      compression: "gzip"

    # Data format
    marshaler: "otlp_json"

    # Custom indexed fields
    indexed_fields:
      - "user.id"
      - "customer.id"
```

## Output Structure

### Data Files

Trace and log telemetry data is stored with the following naming convention:

```
{s3_bucket}/{partition_format}/{signal_type}_{uuid}.{format}[.gz]
```

Examples:
- `telemetry-data/year=2024/month=01/day=15/hour=10/minute=30/traces_abc123.binpb.gz`
- `telemetry-data/year=2024/month=01/day=15/hour=10/minute=30/logs_def456.json.gz`

### Index Files

When indexing is enabled, index files are co-located with data files:

```
{s3_bucket}/{partition_format}/index_{field_name}_{uuid}.{format}[.gz]
```

Examples:
- `telemetry-data/year=2024/month=01/day=15/hour=10/minute=30/index_user.id_ghi789.binpb.gz`
- `telemetry-data/year=2024/month=01/day=15/hour=10/minute=30/index_trace.trace_id_jkl012.json.gz`