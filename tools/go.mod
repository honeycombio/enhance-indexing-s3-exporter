module github.com/honeycombio/enhance-indexing-s3-exporter/tools

go 1.24.0

replace github.com/honeycombio/enhance-indexing-s3-exporter/index => ../index

require (
	github.com/honeycombio/enhance-indexing-s3-exporter/index v0.0.2-alpha
	go.opentelemetry.io/collector/pdata v1.45.0
)

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/hashicorp/go-version v1.7.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	go.opentelemetry.io/collector/featuregate v1.45.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
)
