# Multiple Observers Example

Example demonstrates how multiple `go-sentinel` observers can monitor different tasks or workloads with distinct configurations and metrics. Each observer is optimised for its specific use case and provides separate metrics for monitoring.

## Prerequisites

- Go 1.23 or later
- Docker and Docker Compose (for containerized setup)

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

## Usage

### Local Development

```bash
go run main.go
```

### Docker Compose

```bash
docker compose up -d
```

### Services

- **Application metrics**: http://localhost:8080/metrics
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000


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

## Cleanup

```bash
docker-compose down -v
```
