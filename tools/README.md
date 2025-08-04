# Tools

This directory contains utility tools for working with the enhanced indexing S3 exporter.

## unmarshal-index

A tool to unmarshal and inspect protobuf index files created by the enhanced indexing S3 exporter.

### Usage

```bash
go run unmarshal-index.go <index-file>
```

Or build and run:

```bash
go build -o unmarshal-index .
./unmarshal-index <index-file>
```

### Examples

Unmarshal a protobuf index file:
```bash
./unmarshal-index index_trace_id_abc123.binpb
```

Unmarshal a gzipped protobuf index file:
```bash
./unmarshal-index index_service_name_xyz789.binpb.gz
```

### Output

The tool will output:
1. The field name being indexed
2. The complete index data in JSON format
3. Summary statistics including:
   - Field name
   - Number of unique field values
   - Total number of S3 files referenced

### Sample Output

```
Field Index Contents:
Field Name: trace_id
Index Data:
{
  "field_name": "trace_id",
  "field_index": {
    "abc123def456": {
      "files": [
        "traces-and-logs/year=2025/month=01/day=01/hour=12/minute=30/trace_20250101_123045_uuid1.binpb",
        "traces-and-logs/year=2025/month=01/day=01/hour=12/minute=30/trace_20250101_123046_uuid2.binpb"
      ]
    },
    "def456ghi789": {
      "files": [
        "traces-and-logs/year=2025/month=01/day=01/hour=12/minute=31/trace_20250101_123145_uuid3.binpb"
      ]
    }
  }
}

Summary:
  Field Name: trace_id
  Unique Values: 2
  Total S3 Files: 3
```

## build_proto.sh

This utility generates the functions necessary to marshal an Enhance index into protobuf prior to uploading the index to S3. The structure for the protobuf encoded index file is defined in `index/protos/index.proto`.

To generate the code, run `./build_proto.sh`. This will auto-generate the `index/index.pb.go` file which will in turn be used by the exporter.