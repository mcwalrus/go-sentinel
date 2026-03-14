// Package sentinel provides reliability handling and observability monitoring
// for Go applications. It wraps function execution with automatic Prometheus
// metrics collection, panic recovery, configurable retries, timeouts, and
// graceful shutdown — making critical routines safe, measurable, and reliable.
//
// # Overview
//
// go-sentinel is an observability wrapper: you hand it a function, and it runs
// that function while recording Prometheus metrics for every invocation. You
// choose which metrics to collect via constructor options, so sentinel never
// registers counters or histograms you haven't asked for — avoiding conflicts
// with existing Prometheus registries.
//
// # Core Types
//
// [Observer] wraps a single function and exposes sync ([Observer.Run],
// [Observer.RunFunc]) and async ([Observer.Submit], [Observer.SubmitFunc])
// execution methods. Create one with [NewObserver] for full control over
// which metrics are enabled, or [NewObserverDefault] for a sensible set of
// defaults (in-flight gauge, success counter, error counter).
//
// [VecObserver] is the labeled variant: it manages a family of Observers
// keyed by Prometheus label values, matching the prometheus.GaugeVec /
// prometheus.CounterVec pattern. Retrieve a concrete Observer via
// [VecObserver.With] or [VecObserver.WithLabels].
//
// # Opt-in Metrics
//
// Metrics are enabled at construction time using [ObserverOption] functions:
//
//   - [WithInFlightMetrics]  — gauge of concurrent executions
//   - [WithSuccessMetrics]   — counter of successful completions
//   - [WithErrorMetrics]     — counters for errors, failures, and timeouts
//   - [WithPanicMetrics]     — counter of recovered panics
//   - [WithDurationMetrics]  — histogram of execution durations
//   - [WithRetryMetrics]     — counter of retry attempts
//   - [WithQueueMetrics]     — gauge of queued async tasks
//
// [NewObserverDefault] enables [WithInFlightMetrics], [WithSuccessMetrics],
// and [WithErrorMetrics] for you. Use [NewObserver] when you need a specific
// subset or want to start with no metrics at all.
//
// Every Observer must be registered with a Prometheus registry before metrics
// are collected. Use [Observer.Register] for error handling or
// [Observer.MustRegister] if you prefer a panic on conflicts.
//
// # Async Execution
//
// [Observer.Submit] and [Observer.SubmitFunc] enqueue tasks in an internal
// worker pool (backed by [github.com/sourcegraph/conc]). Call [Observer.Wait]
// to drain the pool and collect all errors as a joined error:
//
//	for _, item := range items {
//	    item := item
//	    observer.Submit(func() error {
//	        return process(item)
//	    })
//	}
//	if err := observer.Wait(); err != nil {
//	    log.Printf("batch errors: %v", err)
//	}
//
// The Observer is reusable after [Observer.Wait] returns. Bound concurrency
// with [WithMaxConcurrency] to cap the goroutine count.
//
// # Retry
//
// Pass a [retry.Retrier] to [WithRetrier] to enable automatic retries on
// error. [retry.DefaultRetrier] provides a configurable implementation with
// pluggable wait strategies (e.g. [retry.Exponential], [retry.Immediate]):
//
//	retrier := retry.NewDefaultRetrier(retry.Exponential(100*time.Millisecond), 3)
//	observer := sentinel.NewObserver(sentinel.WithRetrier(retrier))
//
// Implement the [retry.Retrier] interface to supply custom retry logic.
// Inside the retried function, use [RetryCount] to read the current attempt
// number from the context.
//
// # Graceful Shutdown
//
// [WithControl] accepts a [circuit.Control] function that sentinel calls
// before each execution (and before each retry). Return true to abort.
// [circuit.WhenClosed] wraps a done channel into a Control for clean
// shutdown on signal or context cancellation:
//
//	done := make(chan struct{})
//	// ... close(done) when shutting down
//	observer := sentinel.NewObserver(sentinel.WithControl(circuit.WhenClosed(done)))
//
// # Design Decisions
//
// Opt-in metrics: Prometheus panics when the same metric name is registered
// twice. By requiring explicit opt-in, sentinel never creates metrics you
// haven't asked for, eliminating double-registration conflicts in large
// services that compose multiple libraries.
//
// Retrier interface: Retry logic is delegated to the [retry.Retrier]
// interface rather than built directly into Observer. This makes the retry
// strategy independently testable and swappable without changing call sites.
//
// Structured concurrency via conc: The async pool uses
// [github.com/sourcegraph/conc], which enforces that all goroutines are
// joined before the pool is reused. This prevents goroutine leaks and
// surfaces panics deterministically at [Observer.Wait].
//
// # Quick Start
//
//	package main
//
//	import (
//	    "context"
//	    "fmt"
//	    "log"
//
//	    sentinel "github.com/mcwalrus/go-sentinel"
//	    "github.com/prometheus/client_golang/prometheus"
//	)
//
//	func main() {
//	    // Create a registry and an observer with default metrics.
//	    registry := prometheus.NewRegistry()
//	    observer := sentinel.NewObserverDefault(
//	        sentinel.WithNamespace("myapp"),
//	        sentinel.WithSubsystem("worker"),
//	    )
//	    observer.MustRegister(registry)
//
//	    // Run a function synchronously; metrics are recorded automatically.
//	    err := observer.RunFunc(func(ctx context.Context) error {
//	        fmt.Println("hello from sentinel")
//	        return nil
//	    })
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	}
//
// # Examples
//
// Complete, runnable examples are in the examples/ directory:
//
//   - examples/workerloop  — async Submit()+Wait() batch processing
//   - examples/vec-observer — labeled metrics with VecObserver
//   - examples/multiple-observers — composing multiple observers
package sentinel
