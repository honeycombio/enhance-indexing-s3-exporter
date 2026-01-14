# Enhance Indexing S3 Exporter Changelog

## [Unreleased]

### Added
- Added configurable `usage_reporting_interval` option (only applicable in standalone mode) to control how often usage metrics are sent to Honeycomb API (default: 1m, range: 30s-10m). This helps reduce API rate limiting for high-volume customers.

## [v0.0.9] - 2026-01-12

- maint: bump dependencies to v1.49.0/v0.143.0 (#54) | [Tyler Helmuth](https://github.com/Tyler Helmuth)
- docs: Tidy up all README docs in preparation of GA (#53) | [Josh Parsons](https://github.com/Josh Parsons)
- chore(indexer): Tidy up repo structure (#52) | [Josh Parsons](https://github.com/Josh Parsons)

## [v0.0.8] - 2025-11-12

- fix: xor the partition format validation (#50) | @TylerHelmuth
- refactor(exporter): update fieldS3Keys to be a map for speedier ops (#43) | @asdvalenzuela
- maint: bump collector dependencies (#48) | @TylerHelmuth
- fix: set default queuesize for default batch size (#49) | @TylerHelmuth
- maint: update readme with s3_prefix (#47) | @TylerHelmuth

## [v0.0.7] - 2025-11-06

- chore(indexer): ensure docs and important files are up to date (#45) | @asdvalenzuela
- feat(indexer): Set default batch values in indexer if not present in config (#44) | @jparsons04
- fix: only validate apiendpoint, apikey, and apisecret in standalone mode (#46) | @TylerHelmuth

## [v0.0.6] - 2025-10-31

### Fixed
- Use correct config value for s3Prefix in usage metrics ([#42](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/42))

## [v0.0.5] - 2025-10-31

### Added
- Gather and send usage metrics to Honeycomb in standalone mode ([#41](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/41))

## [v0.0.4] - 2025-10-23

### Added
- Usage metrics collection for the indexer ([#37](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/37))
- Management API key validation ([#35](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/35))
- Hostname configuration option ([#33](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/33))
- Comprehensive README for exporter component with configuration options ([#32](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/32))

### Changed
- Updated configuration options to be config-complete ([#34](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/34))
- Updated configs and README to include management key, secret, and endpoint ([#36](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/36))

### Fixed
- Extract automatic indexes from Resource Attribute fields with defined precedence ([#31](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/31))
- Disallow file_prefix in configuration ([#30](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/30))

## [v0.0.3] - 2025-09-12

### Added
- Automatically index service.name field ([#28](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/28))
- Log signal type support with IndexManager ([#27](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/27))

### Changed
- Renamed automatic trace ID index from `trace_id` to `trace.trace_id` ([#29](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/29))
- Allow minuteIndexBatches to be shared across signal types ([#24](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/24))
- Bump .tool-versions golang to 1.24.6 ([#26](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/26))
- Update .tool-versions to accept any patch version of Go 1.24 ([#25](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/25))

## [v0.0.2] - 2025-08-29

### Added
- Unit tests and configuration validation ([#23](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/23))

### Changed
- Bump OpenTelemetry components to 0.132.0 and Go version to 1.24 ([#22](https://github.com/honeycombio/enhance-indexing-s3-exporter/pull/22))

## [v0.0.1] - 2025-08-20

Initial alpha release of the Enhance Indexing S3 Exporter for OpenTelemetry Collectors.

### Features
- S3 export of OTLP traces and logs in protobuf or JSON format
- Automatic field indexing for trace IDs, service names, and session IDs
- Configurable custom field indexing
- Time-based partitioning with configurable format
- Gzip compression support
- S3-compatible storage support (AWS S3, MinIO, etc.)
- Integration with OpenTelemetry Collector v0.131.0
