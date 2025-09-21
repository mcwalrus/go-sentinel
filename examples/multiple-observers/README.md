# Multiple Observers Example

This example demonstrates how to use multiple `go-sentinel` observers to monitor different types of tasks with distinct configurations and metrics. Each observer is optimized for its specific use case and provides separate metrics for monitoring.

## Architecture

The example includes three different observers:

1. **Background Observer** (`app_background_tasks_*`)
   - Long-running background processing tasks
   - Longer timeouts (30s)
   - Concurrent execution
   - Exponential retry strategy
   - Buckets: [0.1, 0.5, 1, 2, 5, 10, 30, 60] seconds

2. **Critical Observer** (`app_critical_tasks_*`)
   - Time-sensitive business operations
   - Short timeouts (5s)
   - Synchronous execution
   - Immediate retry strategy
   - Buckets: [0.01, 0.1, 0.5, 1, 5, 10, 30] seconds

3. **API Observer** (`app_api_requests_*`)
   - Fast API request processing
   - Very short timeouts (2s)
   - Concurrent execution
   - Minimal retries
   - Buckets: [0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10] seconds

## Task Types

### BackgroundTask
- Simulates processing time: 2-10 seconds
- Failure rate: ~7% (every 15th task)
- Max retries: 3 with exponential backoff

### CriticalTask
- Simulates processing time: 100ms-2s
- Panic rate: ~4% (every 25th task)
- Timeout rate: ~8% (every 12th task)
- Max retries: 2 with immediate retry

### APITask
- Simulates processing time: 10-500ms
- Failure rate: ~5% (every 20th task)
- Max retries: 1 with immediate retry

## Running the Example

### Prerequisites
- Docker and Docker Compose
- Go 1.21+ (for local development)

### Using Docker Compose (Recommended)

1. Start the monitoring stack:
```bash
docker-compose up -d
```

2. Access the services:
   - **Application**: http://localhost:8080/metrics
   - **Prometheus**: http://localhost:9090
   - **Grafana**: http://localhost:3000 (admin/admin)
   - **Alertmanager**: http://localhost:9093

3. Stop the services:
```bash
docker-compose down
```

### Local Development

1. Run the application locally:
```bash
go run main.go
```

2. Start only monitoring services:
```bash
docker-compose up prometheus grafana alertmanager -d
```

## Monitoring and Metrics

### Available Metrics

Each observer exposes the following metrics:

#### Background Tasks (`app_background_tasks_*`)
- `app_background_tasks_duration_seconds` - Histogram of task durations
- `app_background_tasks_total` - Counter of total tasks
- `app_background_tasks_failures_total` - Counter of failed tasks
- `app_background_tasks_retries_total` - Counter of retry attempts
- `app_background_tasks_panics_total` - Counter of panic recoveries
- `app_background_tasks_timeouts_total` - Counter of timeout occurrences
- `app_background_tasks_concurrent_gauge` - Current concurrent tasks

#### Critical Tasks (`app_critical_tasks_*`)
- `app_critical_tasks_duration_seconds` - Histogram of task durations
- `app_critical_tasks_total` - Counter of total tasks
- `app_critical_tasks_failures_total` - Counter of failed tasks
- `app_critical_tasks_retries_total` - Counter of retry attempts
- `app_critical_tasks_panics_total` - Counter of panic recoveries
- `app_critical_tasks_timeouts_total` - Counter of timeout occurrences
- `app_critical_tasks_concurrent_gauge` - Current concurrent tasks

#### API Requests (`app_api_requests_*`)
- `app_api_requests_duration_seconds` - Histogram of request durations
- `app_api_requests_total` - Counter of total requests
- `app_api_requests_failures_total` - Counter of failed requests
- `app_api_requests_retries_total` - Counter of retry attempts
- `app_api_requests_panics_total` - Counter of panic recoveries
- `app_api_requests_timeouts_total` - Counter of timeout occurrences
- `app_api_requests_concurrent_gauge` - Current concurrent requests

### Prometheus Queries

Here are some useful Prometheus queries to monitor the different observers:

#### Success Rate by Observer Type
```promql
rate(app_background_tasks_total[5m]) - rate(app_background_tasks_failures_total[5m])
rate(app_critical_tasks_total[5m]) - rate(app_critical_tasks_failures_total[5m])
rate(app_api_requests_total[5m]) - rate(app_api_requests_failures_total[5m])
```

#### Average Duration by Observer Type
```promql
rate(app_background_tasks_duration_seconds_sum[5m]) / rate(app_background_tasks_duration_seconds_count[5m])
rate(app_critical_tasks_duration_seconds_sum[5m]) / rate(app_critical_tasks_duration_seconds_count[5m])
rate(app_api_requests_duration_seconds_sum[5m]) / rate(app_api_requests_duration_seconds_count[5m])
```

#### 95th Percentile Duration
```promql
histogram_quantile(0.95, rate(app_background_tasks_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(app_critical_tasks_duration_seconds_bucket[5m]))
histogram_quantile(0.95, rate(app_api_requests_duration_seconds_bucket[5m]))
```

#### Concurrent Tasks
```promql
app_background_tasks_concurrent_gauge
app_critical_tasks_concurrent_gauge
app_api_requests_concurrent_gauge
```

### Grafana Dashboards

The example includes Grafana with pre-configured dashboards (if you add the dashboard configs). You can create custom dashboards to visualize:

1. **Task Performance Overview**
   - Success rates across all observers
   - Average durations
   - Error rates and types

2. **Observer Comparison**
   - Side-by-side metrics for different observers
   - Throughput comparison
   - Resource utilization

3. **Error Analysis**
   - Failure rates by task type
   - Retry patterns
   - Panic and timeout frequencies

## Configuration

### Observer Configuration

Each observer can be configured independently:

```go
backgroundObserver = sentinel.NewObserver(sentinel.ObserverConfig{
    Namespace:       "app",
    Subsystem:       "background_tasks",
    Description:     "Background processing tasks",
    BucketDurations: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
})
```

### Task Configuration

Each task type has its own configuration:

```go
func (task *BackgroundTask) Config() sentinel.TaskConfig {
    return sentinel.TaskConfig{
        Timeout:       30 * time.Second,
        Concurrent:    true,
        RecoverPanics: true,
        MaxRetries:    3,
        RetryStrategy: sentinel.RetryStrategyExponential,
    }
}
```

## Use Cases

This pattern is useful when you have:

1. **Different SLA requirements** - API requests need faster response times than background jobs
2. **Different retry strategies** - Critical tasks might need immediate retries while background tasks can use exponential backoff
3. **Different monitoring needs** - Each task type might need different histogram buckets
4. **Resource isolation** - Separate observers allow for independent scaling and monitoring

## Extending the Example

You can extend this example by:

1. Adding more observer types (e.g., batch processing, data sync, notifications)
2. Implementing custom retry strategies
3. Adding more sophisticated task routing
4. Integrating with external systems (databases, message queues, etc.)
5. Adding custom metrics and labels
6. Implementing circuit breakers or rate limiting per observer

## Troubleshooting

### Common Issues

1. **Port conflicts**: Make sure ports 8080, 9090, 3000, and 9093 are available
2. **Docker issues**: Ensure Docker daemon is running and you have sufficient resources
3. **Module issues**: Run `go mod tidy` if you encounter dependency problems

### Logs

The application provides detailed logging for each task execution:
- `[BACKGROUND]` - Background task logs
- `[CRITICAL]` - Critical task logs  
- `[API]` - API task logs

### Health Checks

The Docker container includes health checks on the `/metrics` endpoint. You can also manually check:

```bash
curl http://localhost:8080/metrics
```

This should return Prometheus-formatted metrics for all three observers.
