package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	exporterpkg "github.com/honeycombio/enhance-indexing-s3-exporter/indexings3exporter"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/exporter"
	debugpkg "go.opentelemetry.io/collector/exporter/debugexporter"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config/local.yaml", "Path to the configuration file")
	flag.Parse()

	factories, err := getFactories()
	if err != nil {
		log.Fatalf("Failed to get factories: %v", err)
	}

	settings := otelcol.CollectorSettings{
		Factories: func() (otelcol.Factories, error) {
			return factories, nil
		},
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: []string{configPath},
				ProviderFactories: []confmap.ProviderFactory{
					fileprovider.NewFactory(),
				},
			},
		},
	}

	collector, err := otelcol.NewCollector(settings)
	if err != nil {
		log.Fatalf("Failed to create collector: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		fmt.Println("Shutting down collector...")
		cancel()
	}()

	if err := collector.Run(ctx); err != nil {
		log.Fatalf("Collector run failed: %v", err)
	}
}

func getFactories() (otelcol.Factories, error) {
	var factories otelcol.Factories

	// Receivers
	factories.Receivers = map[component.Type]receiver.Factory{
		otlpreceiver.NewFactory().Type(): otlpreceiver.NewFactory(),
	}

	// Exporters
	factories.Exporters = map[component.Type]exporter.Factory{
		exporterpkg.NewFactory().Type(): exporterpkg.NewFactory(),
		debugpkg.NewFactory().Type():    debugpkg.NewFactory(),
	}

	// Processors (empty for now)
	factories.Processors = map[component.Type]processor.Factory{}

	// Extensions (empty for now)
	factories.Extensions = map[component.Type]extension.Factory{}

	return factories, nil
}
