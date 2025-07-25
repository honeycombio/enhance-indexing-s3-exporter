FROM golang:1.24 as builder
ENV AWS_ACCESS_KEY_ID=minioadmin
ENV AWS_SECRET_ACCESS_KEY=minioadmin
WORKDIR /app
COPY enhanceindexings3exporter ./enhanceindexings3exporter
COPY cmd/otelcol ./cmd/otelcol
WORKDIR /app/cmd/otelcol
RUN go mod download
RUN go build -o otelcol main.go

FROM debian:stable-slim
WORKDIR /otelcol

COPY --from=builder /app/cmd/otelcol/otelcol /otelcol

COPY config/local.yaml ./config/local.yaml

EXPOSE 4317

ENTRYPOINT ["/otelcol/otelcol"]