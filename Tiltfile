# Tiltfile for OpenTelemetry Collector Builder (OCB) Development

# Configuration
COLLECTOR_NAME = "otelcol-dev"
CONFIG_PATH = "./config/local.yaml"
BUILDER_CONFIG = "./builder-config.yaml"
OUTPUT_DIR = "./otelcol-dev"

# Watch for changes in the custom exporter and related files
watch_file('./builder-config.yaml')
watch_file('./enhanceindexings3exporter/')
watch_file('./config/')

# Build the collector using OCB
local_resource(
    'ocb-build',
    cmd=[
        'builder',
        '--config=' + BUILDER_CONFIG,
    ],
    deps=[
        './builder-config.yaml',
        './enhanceindexings3exporter/',
    ],
    labels=["build"],
)

# Run the built collector
local_resource(
    'otelcol-run',
    serve_cmd=[
        OUTPUT_DIR + '/' + COLLECTOR_NAME,
        '--config=' + CONFIG_PATH,
    ],
    deps=[OUTPUT_DIR],
    resource_deps=['ocb-build'],
    labels=["collector"],
    serve_env={'GOMEMLIMIT': '100MiB'},  # Optional: limit memory usage
)

# Set up port forwarding for OTLP endpoints
local_resource(
    'port-forwards',
    cmd=['echo', 'Port forwarding: 4317 (gRPC), 4318 (HTTP)'],
    resource_deps=['otelcol-run'],
    auto_init=False,
    labels=["network"],
)

# Optional: Add a resource to validate the configuration
local_resource(
    'validate-config',
    cmd=[
        OUTPUT_DIR + '/' + COLLECTOR_NAME,
        '--config=' + CONFIG_PATH,
        '--version',
    ],
    deps=[CONFIG_PATH, OUTPUT_DIR],
    resource_deps=['ocb-build'],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    labels=["validation"],
)

# Optional: Clean build artifacts
local_resource(
    'clean',
    cmd=['rm', '-rf', OUTPUT_DIR],
    auto_init=False,
    trigger_mode=TRIGGER_MODE_MANUAL,
    labels=["cleanup"],
)

# Set up log forwarding for better debugging
local_resource(
    'logs',
    serve_cmd=['tail', '-f', '/dev/null'],  # Placeholder for log aggregation
    resource_deps=['otelcol-run'],
    auto_init=False,
    labels=["logs"],
)

print("🚀 OpenTelemetry Collector Builder Development Environment")
print("📋 Available resources:")
print("   • ocb-build: Builds the collector using OCB")
print("   • otelcol-run: Runs the built collector")
print("   • validate-config: Validates collector configuration (manual)")
print("   • clean: Cleans build artifacts (manual)")
print("")
print("🔗 Network:")
print("   • Collector runs on ports 4317 (gRPC) and 4318 (HTTP)")
print("   • Access endpoints at localhost:4317 and localhost:4318")
print("")
print("💡 Tips:")
print("   • Edit files in ./enhanceindexings3exporter/ to trigger rebuilds")
print("   • Modify ./config/local.yaml to change collector configuration")
print("   • Use 'tilt trigger validate-config' to validate config")
print("   • Use 'tilt trigger clean' to clean build artifacts") 