# Reliable

Provides task reliablity features and obserability monitoring via exported Prometheus metrics.

Features

1. Promethesus metrics of worker pool.
3. Provides visibility and logging of worker errors.
4. Provides retry strategies and timeouts.
6. Logging integration.

Go routines return errors, if an error occurs within a routine, the reliablity handler will notify the promthesus endpoint.

Promthesus metrics:

- In Flight
- Successes
- Error count
- Timeout Errors
- Panics Occurances
- Routine Runtime Histogram
- Retries

Retries can be configured per worker. These are useful.