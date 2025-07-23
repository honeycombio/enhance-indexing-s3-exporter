# OpenTelemetry Collector Builder with Tilt

This project uses [Tilt](https://tilt.dev/) to streamline development with the OpenTelemetry Collector Builder (OCB).

## 🚀 Quick Start

1. **Start the development environment:**
   ```bash
   tilt up
   ```

2. **Access the Tilt dashboard:**
   - Open your browser to: http://localhost:10350/
   - Monitor build status, logs, and resource health

3. **Send test data to your collector:**
   ```bash
   # OTLP gRPC endpoint
   curl -X POST http://localhost:4317/v1/traces
   
   # OTLP HTTP endpoint  
   curl -X POST http://localhost:4318/v1/traces
   ```

## 📋 Available Resources

| Resource | Description | Type |
|----------|-------------|------|
| `ocb-build` | Builds the collector using OCB | Auto |
| `otelcol-run` | Runs the built collector | Auto |
| `validate-config` | Validates collector configuration | Manual |
| `clean` | Cleans build artifacts | Manual |
| `port-forwards` | Network information | Manual |

## 🔧 Manual Commands

```bash
# Validate configuration
tilt trigger validate-config

# Clean build artifacts
tilt trigger clean

# Stop all resources
tilt down
```

## 🛠️ Development Workflow

1. **Edit your exporter:** Modify files in `./enhanceindexings3exporter/`
   - Tilt automatically detects changes and rebuilds

2. **Update configuration:** Edit `./config/local.yaml`
   - Collector restarts with new configuration

3. **Modify build config:** Edit `./builder-config.yaml`
   - Triggers a full OCB rebuild

## 🔗 Endpoints

- **OTLP gRPC:** `localhost:4317`
- **OTLP HTTP:** `localhost:4318`
- **Tilt Dashboard:** `localhost:10350`

## 📊 Monitoring

View real-time logs and metrics in the Tilt dashboard:
- Build progress and status
- Collector startup logs
- Export activity (to S3 and debug)
- Resource health checks

## 🐛 Troubleshooting

1. **Build failures:** Check the `ocb-build` resource logs in Tilt
2. **Runtime errors:** Monitor `otelcol-run` resource logs
3. **Config issues:** Use `tilt trigger validate-config`
4. **Clean slate:** Run `tilt trigger clean` then restart

## 💡 Tips

- Keep the Tilt dashboard open for real-time feedback
- Use the resource labels to filter and organize views
- Check logs regularly during development
- The collector includes both your custom exporter and debug output 