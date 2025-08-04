FROM golang:1.24 as builder
ENV AWS_ACCESS_KEY_ID=minioadmin
ENV AWS_SECRET_ACCESS_KEY=minioadmin
ENV GOPRIVATE=github.com/honeycombio/enhance-indexing-s3-exporter
ENV GOPROXY=direct
ENV GOSUMDB=off
RUN go env -w GOCACHE=/go-cache
RUN go env -w GOMODCACHE=/gomod-cache
WORKDIR /app
COPY go.work go.work.sum* ./
COPY enhanceindexings3exporter ./enhanceindexings3exporter
COPY cmd/otelcol ./cmd/otelcol
COPY index ./index
WORKDIR /app/cmd/otelcol
RUN --mount=type=cache,target=/go-cache --mount=type=cache,target=/gomod-cache \
  go mod download
RUN --mount=type=cache,target=/go-cache --mount=type=cache,target=/gomod-cache \
  go build -o otelcol main.go

FROM debian:stable-slim
WORKDIR /otelcol

COPY --from=builder /app/cmd/otelcol/otelcol /otelcol

COPY config/local.yaml ./config/local.yaml

EXPOSE 4317

ENTRYPOINT ["/otelcol/otelcol"]