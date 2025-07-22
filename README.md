# enhance-indexing-s3-exporter

## how to test

1. Start the collector

```go
go run ./cmd/otelcol --config config/local.yaml
```

2. Use a version of `otelgen` that can communicate with localhost port 4317. The direct docker version of otelgen may not work, you may need to install that via brew first

```curl
brew install krzko/tap/otelgen
```

Send a single log through the collector:

```curl
otelgen --otel-exporter-otlp-endpoint localhost:4317 --insecure logs single
```
