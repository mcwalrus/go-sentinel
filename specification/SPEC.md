# taskscope

Provides task reliability features and observability monitoring via exported Prometheus metrics.    

Features

1. Prometheus metrics of tasks / jobs.
3. Visibility of task / job errors.
4. Provides retry strategies and timeouts.

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