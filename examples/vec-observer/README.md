# VecObserver Example

This example demonstrates the use of `VecObserver` to create multiple child observers that share underlying metrics but are differentiated by Prometheus labels.

## Running the Example

```bash
cd examples/vec-observer
docker compose -f docker-compose.yml up
```

The application will:
- Start a metrics server on `:8080/metrics`
- Simulate tasks running with different observers (api-prod, api-staging, db-prod)
- Each observer maintains its own configuration while contributing to shared metrics

## Viewing Metrics

Visit `http://localhost:8080/metrics` to see Prometheus metrics with labels:

```
example_vec_successes_total{environment="production",service="api"} 42
example_vec_successes_total{environment="staging",service="api"} 38
example_vec_successes_total{environment="production",service="database"} 15
```

## Key Concepts

- **VecObserver**: Creates observers that support Prometheus labels for multi-dimensional metrics
- **WithLabels()**: Creates child observers with specific label values
- **Shared Metrics**: All child observers contribute to the same metric time series, differentiated by labels
- **Independent Configuration**: Each child observer can have its own timeout, retry, and other settings

