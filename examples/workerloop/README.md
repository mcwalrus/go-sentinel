# Worker Loop Example with Prometheus Metrics

This example demonstrates how to use the go-sentinel library to observe worker tasks and expose Prometheus metrics.

## Prerequisites

- Go 1.23 or later
- Docker and Docker Compose (for containerized setup)

## Usage

### Local Development

```bash
go run main.go
```

Access metrics at: http://localhost:8080/metrics

### Docker Compose

```bash
docker-compose up -d
```

Access:
- **Application metrics**: http://localhost:8080/metrics
- **Prometheus**: http://localhost:9090
- **Grafana**: http://localhost:3000 (admin/admin)

## Exposed Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `example_workerloop_in_flight` | Gauge | Tasks currently running |
| `example_workerloop_successes` | Counter | Total successful tasks |
| `example_workerloop_errors` | Counter | Total failed tasks |
| `example_workerloop_timeout_errors` | Counter | Total timeout errors |
| `example_workerloop_panic_occurances` | Counter | Total panics |
| `example_workerloop_observed_duration` | Histogram | Task execution time |
| `example_workerloop_attempted_retry` | Counter | Total retry attempts |

## Cleanup

```bash
docker-compose down -v
```
