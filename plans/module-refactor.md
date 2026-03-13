I have Observer and VecObserver as groups to observe functions and record metrics.
I want to make a refactor to the library and am comfortable with making breaking changes to the API.
I intend to increment the major version of the library on the back of this.

The key cases the user needs optionality for:
* Recording Retry attempts.
* Recording Panic occurances.
* Recording durations.
* Recording timouts.
* Recording Queue/Async metrics.

Given the current pattern, this might look something like.

```Go
// Opt-in, composable builder
observer := sentinel.NewObserver(
    sentinel.WithInFlightMetrics(), // in-flight
    sentinel.WithSuccessMetrics(),      // success
    sentinel.WithErrorMetrics(),        // errors
    sentinel.WithPanicMetrics(),        // panics_total
    sentinel.WithRetryMetrics(),        // retries_total + failures
    sentinel.WithTimeoutMetrics(),      // timeouts_total
    sentinel.WithDurationMetrics(       // durations_seconds histogram
        []float64{0.1, 0.25, 0.5, 1.0},
    ),
    sentinel.WithQueueMetrics(),        // in_flight + pending_total
)
```

There might even be options to create variants of the same metric based on how they are handled.
I.e errors could be labelled by error type as they come through, which could be defined through the API.
Builders pattern for the observer might be better. Users might also create metrics based on custom conditions.


**Guidance for the agent:** Give it `observer.go`, `observer_config.go`, `vec_observer.go`, `metrics.go`, and all `*_test.go` files. Instruct it to preserve backward compatibility via a `NewObserverDefault()` that opts into success, inflight, and error metrics. 

Other requirements:
* Use `github.com/sourcegraph/conc` or nested librarires for concurrency management.
* Observers should be setup to be composable through builder methods with options for metrics exported.
* Keep Observer as a sub-set of VecObserver with their overlapping interfaces inline.

Maintain `Run()` as remain synchronous.
Introduce async `Submit()` calls to observers.

Areas for improvement:
- Godoc comments on every exported types and methods.
- A `doc.go` with a package-level overview and decision rationale.
- `UseConfig` could be implemented differently on construction of the observers.

Keep retry, remove or demote circuit. Use something like:

```go
type Retrier interface {
    Do(ctx context.Context, fn func() error) error
}
```

The handlers should be allowed to be built with custom mechanisms for retry handling / concurrency request limiting, etc.
Improve the documentation so that anyone / AI can understand the nature of the library easily.
The application should manaage panic handling across all boundaries.
