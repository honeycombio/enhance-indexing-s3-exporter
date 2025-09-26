# Enhance Indexing S3 Exporter Configuration

This exporter extends the OpenTelemetry AWS S3 exporter with automatic field indexing capabilities. It writes telemetry data to S3 while simultaneously generating field-based indexes for efficient querying.

## Supported Configuration Options

### Core Configuration Structure

```yaml
exporters:
   enhance_indexing_s3_exporter:
    # Standard OpenTelemetry exporter settings
    sending_queue: # Queue configuration
    timeout: # Timeout settings  
    retry_on_failure: # Retry configuration
    
    # S3 uploader configuration (required)
    s3uploader:
      # ... S3 settings
    
    # Data marshaling format (required) 
    marshaler: "otlp_protobuf"  # or "otlp_json"
    
    # Index configuration (optional)
    index:
      enabled: true
      indexed_fields: ["user.id", "customer.id"]
```

### S3 Uploader Configuration (`s3uploader`)

| Field | Description | Default |
|-------|-------------|---------|
| `region` | AWS region (required) | "us-east-1" |
| `s3_bucket` | S3 bucket (required unless `endpoint` provided) | - |
| `s3_partition_format` | Filepath partitioning format (see [strftime][1]). Must contain year/month/day/hour/minute placeholders. | "year=%Y/month=%m/day=%d/hour=%H/minute=%M" |
| `compression` | File compression: "gzip" or "none" | "gzip" |
| `retry_mode` | Retry strategy: "standard" or "adaptive" | "standard" |
| `retry_max_attempts` | Maximum retry attempts | 3 |
| `endpoint` | Custom S3 endpoint (for S3-compatible services) | - |
| `s3_force_path_style` | Force path-style S3 addressing | false |
| `disable_ssl` | Disable SSL for S3 requests | false |

[1]: https://www.man7.org/linux/man-pages/man3/strftime.3.html

**Note**: `file_prefix` is **not supported** and will cause validation to fail.

### Marshaler Configuration

The `marshaler` field determines the data format written to S3:

- **`otlp_protobuf`** (default): OpenTelemetry Protocol as Protocol Buffers - more efficient, smaller files
- **`otlp_json`**: OpenTelemetry Protocol as JSON - human-readable format

### Index Configuration (`index`)

| Field | Description | Default |
|-------|-------------|---------|
| `enabled` | Enable automatic field indexing | false |
| `indexed_fields` | Array of custom field names to index | [] |

#### Automatically Indexed Fields

When indexing is enabled (`index.enabled: true`), these fields are **automatically indexed**:

- **`trace.trace_id`**: Trace identifier (from span/log trace ID)
- **`service.name`**: Service name (from resource/scope/item attributes)  
- **`session.id`**: Session identifier (from resource/scope/item attributes)

#### Custom Indexed Fields

Additional fields can be configured for indexing:

```yaml
index:
  enabled: true
  indexed_fields:
    - "user.id"
    - "customer.id"
    - "environment" 
    - "version"
```

#### Field Value Precedence

When the same field appears in multiple locations, values are resolved with the following precedence (highest to lowest):

1. **Item attributes** (span attributes for traces, log record attributes for logs)
2. **Instrumentation scope attributes**  
3. **Resource attributes**

#### Index File Output

Index files are generated with this naming pattern:
```
{partition_path}/index_{field_name}_{uuid}.{format}[.gz]
```

Examples:
- `year=2024/month=01/day=15/hour=10/minute=30/index_user.id_abc123.json.gz`
- `year=2024/month=01/day=15/hour=10/minute=30/index_trace.trace_id_def456.binpb.gz`

### Configuration Validation

The exporter validates configuration with these rules:

- ✅ `region` is required
- ✅ `s3_bucket` is required unless `endpoint` is provided  
- ✅ `compression` must be "gzip" or "none"
- ✅ `retry_mode` must be "standard" or "adaptive"
- ✅ `marshaler` must be "otlp_json" or "otlp_protobuf"
- ✅ `s3_partition_format` must contain year/month/day/hour/minute placeholders
- ✅ `s3_partition_format` cannot start or end with "/"
- ❌ `file_prefix` is not supported (will cause validation failure)

## Example Configurations

### Basic Configuration

```yaml
exporters:
  enhance_indexing_s3_exporter:
    s3uploader:
      region: "us-west-2"
      s3_bucket: "telemetry-data"
      s3_partition_format: "year=%Y/month=%m/day=%d/hour=%H/minute=%M"
      compression: "gzip"
    marshaler: "otlp_protobuf"
    index:
      enabled: false
```

### Configuration with Indexing

```yaml
exporters:
  enhance_indexing_s3_exporter:
    # Queue and timeout settings
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
    index:
      enabled: true
      indexed_fields:
        - "user.id"
        - "customer.id"
        - "environment"
        - "deployment.version"

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
  enhance_indexing_s3:
    s3uploader:
      region: "us-east-1"
      endpoint: "http://localhost:9000"
      s3_bucket: "telemetry-bucket"
      s3_force_path_style: true
      disable_ssl: true
      s3_partition_format: "year=%Y/month=%m/day=%d/hour=%H/minute=%M"
      compression: "gzip"
    marshaler: "otlp_json"
    index:
      enabled: true
      indexed_fields:
        - "user.id"
        - "session.id"
```

## Output Structure

### Data Files

Telemetry data is stored in the following path format:

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