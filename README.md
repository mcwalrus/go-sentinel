# Sentinel

Sentinel provides reliability with observability monitoring for tasks in Go applications. It wraps task execution with Prometheus metrics, error handling, retries, and timeouts — making background jobs and goroutines safe, measurable, and reliable.


sentinel_in_flight	Current number of running tasks
sentinel_success_total	Counter of successful task completions
sentinel_errors_total	Counter of failed tasks
sentinel_timeout_total	Counter of timed-out tasks
sentinel_panics_total	Counter of panics recovered in tasks
sentinel_runtime_seconds	Histogram of task execution durations
sentinel_retries_total	Counter of retries attempted


Roadmap:

[-] Labels support, Vec support.
[-] Curcuit breaker with supported prom metrics.
    - https://medium.com/@homayoonalimohammadi/circuitbreakers-in-go-d85f5297cb50
