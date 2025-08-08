package enhanceindexings3exporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func TestConfigMerging(t *testing.T) {
	t.Run("session.id is always in indexed fields", func(t *testing.T) {
		var exp *enhanceIndexingS3Exporter
		createTracesExporter := func(ctx context.Context, set exporter.Settings, cfg component.Config) (exporter.Traces, error) {
			// Only create the exporter once.
			if exp == nil {
				var err error
				exp, err = newEnhanceIndexingS3Exporter(cfg.(*Config), set.Logger)
				if err != nil {
					return nil, err
				}
			}

			return exporterhelper.NewTraces(ctx, set, cfg, exp.consumeTraces)
		}

		factory := func() exporter.Factory {
			return exporter.NewFactory(
				component.MustNewType("test"),
				createDefaultConfig,
				exporter.WithTraces(createTracesExporter, stability),
			)
		}

		settings := otelcol.CollectorSettings{
			Factories: func() (factories otelcol.Factories, err error) {
				factories.Exporters = map[component.Type]exporter.Factory{
					factory().Type(): factory(),
				}

				factories.Receivers = map[component.Type]receiver.Factory{
					otlpreceiver.NewFactory().Type(): otlpreceiver.NewFactory(),
				}

				return factories, nil
			},
			ConfigProviderSettings: otelcol.ConfigProviderSettings{
				ResolverSettings: confmap.ResolverSettings{
					URIs: []string{
						"yaml:exporters::test::index::enabled: true",
						"yaml:exporters::test::index::indexed_fields: [abc]",

						"yaml:exporters::test::s3uploader::s3_bucket: mybucket",
						"yaml:receivers::otlp::protocols::grpc::endpoint: 0.0.0.0:4317",
						"yaml:service::pipelines::traces/objectstorage::receivers: [otlp]",
						"yaml:service::pipelines::traces/objectstorage::exporters: [test]",
					},
					ProviderFactories: []confmap.ProviderFactory{
						yamlprovider.NewFactory(),
					},
				},
			},
		}

		col, err := otelcol.NewCollector(settings)
		require.NoError(t, err)

		err = col.DryRun(t.Context())
		require.NoError(t, err)

		assert.NotNil(t, exp.config)
		assert.Contains(t, exp.config.IndexConfig.IndexedFields, fieldName("session.id"))
	})
}
