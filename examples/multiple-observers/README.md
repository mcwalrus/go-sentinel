# Multiple Observers Example

This example demonstrates how to use multiple `go-sentinel` observers to monitor different types of tasks with distinct configurations and metrics. Each observer is optimized for its specific use case and provides separate metrics for monitoring.

### Prerequisites
- Docker and Docker Compose
- Go 1.23+ (for local development)

## Task Types

1. **Background Observer** (`app_background_tasks_*`)
- Simulates processing time: 2-10 seconds
- Failure rate: ~7% (every 15th task)
- Max retries: 3 with exponential backoff

2. **Critical Observer** (`app_critical_tasks_*`)
- Simulates processing time: 100ms-2s
- Panic rate: ~4% (every 25th task)
- Timeout rate: ~8% (every 12th task)
- Max retries: 2 with immediate retry

3. **API Observer** (`app_api_requests_*`)
- Simulates processing time: 10-500ms
- Failure rate: ~5% (every 20th task)
- Max retries: 1 with immediate retry

### Docker Compose

```bash
docker-compose up -d
```

### Services 
   - **Application**: http://localhost:8080/metrics
   - **Prometheus**: http://localhost:9090
   - **Grafana**: http://localhost:3000

### Docker Cleanup

```bash
docker-compose down
```

### Prometheus Queries

Here are some useful Prometheus queries to monitor the different observers:

#### Success Rate by Observer Type
```promql
rate(app_background_tasks_successes_total[5m]) - rate(app_background_tasks_errors_total[5m])
rate(app_critical_tasks_successes_total[5m]) - rate(app_critical_tasks_errors_total[5m])
rate(app_api_requests_successes_total[5m]) - rate(app_api_requests_errors_total[5m])
```

#### Average Duration by Observer Type
```promql
rate(app_background_tasks_durations_seconds_sum[5m]) / rate(app_background_tasks_durations_seconds_count[5m])
rate(app_critical_tasks_durations_seconds_sum[5m]) / rate(app_critical_tasks_durations_seconds_count[5m])
rate(app_api_requests_durations_seconds_sum[5m]) / rate(app_api_requests_durations_seconds_count[5m])
```

#### 95th Percentile Duration
```promql
histogram_quantile(0.95, rate(app_background_tasks_durations_seconds_bucket[5m]))
histogram_quantile(0.95, rate(app_critical_tasks_durations_seconds_bucket[5m]))
histogram_quantile(0.95, rate(app_api_requests_durations_seconds_bucket[5m]))
```

### Logs

The application provides detailed logging for each task execution:
- `[BACKGROUND]` - Background task logs
- `[CRITICAL]` - Critical task logs  
- `[API]` - API task logs

### Health Check

The Docker container includes health checks on the `/metrics` endpoint. You can also manually check:

```bash
curl http://localhost:8080/metrics
```

This should return Prometheus-formatted metrics for all three observers.
