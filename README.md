# enhance-indexing-s3-exporter

## local development

### 1. Start Docker Daemon

Before running the project, ensure that the Docker daemon (dockerd) is running on your system.

### 2. Start the Project with Tilt

Use Tilt to start and manage the development environment. In your project directory, run:
```tilt up```

### 3. Send test signals

Open your browser and navigate to `http://localhost:10350`. Use one of the available generators to send logs or traces:

- `otelgen_one_log`
- `otelgen_one_trace`
- `otelgen_logs_stream`
- `otelgen_traces_stream`

### 4. Examine the locally uploaded file

To view the locally uploaded files, go to `http://localhost:9001`.
Log in with the following credentials:

- Username: `minioadmin`
- Password: `minioadmin`

Browse the buckets to verify the uploaded data.

