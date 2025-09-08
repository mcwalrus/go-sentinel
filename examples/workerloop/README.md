# Worker Loop Example with Prometheus Metrics

This example demonstrates how to use the go-sentinel library to observe worker tasks and expose Prometheus metrics for monitoring.

## Features

- **HTTP Metrics Server**: Exposes Prometheus metrics on port 8080
- **Docker Support**: Complete containerization with Docker Compose
- **Monitoring Stack**: Includes Prometheus and Grafana for visualization
- **Graceful Shutdown**: Proper signal handling and cleanup

## Metrics Exposed

The application exposes the following Prometheus metrics:

- `example_workerloop_in_flight`: Number of tasks currently in flight
- `example_workerloop_successes`: Total number of successful tasks
- `example_workerloop_errors`: Total number of failed tasks
- `example_workerloop_timeout_errors`: Total number of timeout errors
- `example_workerloop_panic_occurances`: Total number of panics
- `example_workerloop_observed_duration`: Histogram of task execution durations
- `example_workerloop_attempted_retry`: Total number of retry attempts

## Running Locally

### Prerequisites

- Go 1.25.0 or later
- Docker and Docker Compose (optional)

### Direct Execution

```bash
go run main.go
```

The application will:
1. Start a metrics server on port 8080
2. Process initial batch jobs
3. Continue processing periodic jobs every 10 seconds
4. Run until you press Ctrl+C

Access metrics at: http://localhost:8080/metrics

### Docker Compose (Recommended)

Run the complete monitoring stack:

```bash
docker-compose up -d
```

This will start:
- **Worker Loop Application**: http://localhost:8080
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin)

### Monitoring

1. **Prometheus UI**: Navigate to http://localhost:9090
   - View metrics and create queries
   - Check targets status at `/targets`

2. **Grafana Dashboard**: Navigate to http://localhost:3000
   - Login with admin/admin
   - Add Prometheus as data source: http://prometheus:9090
   - Create dashboards to visualize worker metrics

### Example Queries

Some useful Prometheus queries:

```promql
# Current tasks in flight
example_workerloop_in_flight

# Success rate over 5 minutes
rate(example_workerloop_successes[5m])

# Error rate over 5 minutes  
rate(example_workerloop_errors[5m])

# Average task duration
rate(example_workerloop_observed_duration_sum[5m]) / rate(example_workerloop_observed_duration_count[5m])

# 95th percentile task duration
histogram_quantile(0.95, rate(example_workerloop_observed_duration_bucket[5m]))
```

## Configuration

### Prometheus Scraping

The Prometheus configuration (`prometheus.yml`) scrapes metrics from the worker loop every 5 seconds:

```yaml
- job_name: 'workerloop'
  static_configs:
    - targets: ['workerloop:8080']
  metrics_path: '/metrics'
  scrape_interval: 5s
```

### Observer Configuration

The sentinel observer is configured with custom buckets optimized for the expected task durations:

```go
ob = sentinel.NewObserver(sentinel.ObserverConfig{
    Namespace:   "example",
    Subsystem:   "workerloop", 
    Description: "Worker loop",
    Buckets:     []float64{0.01, 0.1, 1, 10, 100, 1000, 10_000},
})
```

## Cleanup

Stop and remove all containers:

```bash
docker-compose down -v
```

This removes containers and volumes (including stored metrics data).
