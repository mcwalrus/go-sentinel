// Package sentinel provides reliability handling and observability monitoring for Go applications.
// It wraps task execution with Prometheus metrics, observing errors, panic occurrences, retries,
// and timeouts — making critical routines safe, measurable, and reliable.
package sentinel

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Observer monitors and measures task executions, collecting Prometheus metrics
// for successes, failures, timeouts, panics, retries, and observed runtimes.
// It provides methods to execute tasks with various TaskConfig options.
type Observer struct {
	cfg           observerConfig
	metrics       *metrics
	recoverPanics bool
	runner        RunConfig
}

// NewObserver configures a new Observer with specified prometheus metrics.
// The Observer will need to be registered with a Prometheus registry to expose metrics.
// Please refer to [Observer.MustRegister] and [Observer.Register] for more information.
//
// Example usage:
//
//	// Uses default configuration
//	observer := sentinel.NewObserver()
//
//	// Replaces default "sentinel"
//	observer := sentinel.NewObserver(sentinel.WithSubsystem("workers"))
//
//	// Support all metrics
//	observer := sentinel.NewObserver(
//	  sentinel.WithRetryMetrics(),
//	  sentinel.WithTimeoutMetrics(),
//	  sentinel.WithDurationMetrics([]float64{0.05, 1, 5, 30, 600}),
//	)
func NewObserver(opts ...PrometheusOption) *Observer {
	cfg := observerConfig{
		namespace:   "",
		subsystem:   "",
		description: "tasks",
	}

	// Apply options
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	// Apply defaults if not set
	if cfg.subsystem == "" && cfg.namespace == "" {
		cfg.subsystem = "sentinel"
	}
	if cfg.description == "" {
		cfg.description = "tasks"
	}

	return &Observer{
		cfg:     cfg,
		metrics: newMetrics(cfg),
	}
}

// TODO: redo docs.
// Register registers all Observer metrics with the provided Prometheus registry.
// Use [Observer.MustRegister] if you want the program to panic on registration conflicts.
func (o *Observer) Register(registry prometheus.Registerer) error {
	return o.metrics.Register(registry)
}

// TODO: redo docs.
// MustRegister registers all Observer metrics with the provided Prometheus registry.
// This method panics if any metric registration failures. Use [Observer.Register] if you prefer
// to handle registration errors gracefully.
func (o *Observer) MustRegister(registry prometheus.Registerer) {
	o.metrics.MustRegister(registry)
}

// TODO: redo docs.
// UseRunConfig configures the observer with a different set of RunConfig.
// It allows the observer to be configured with different options for observer Run methods.
// They create new observers with the same underlying registered metrics, but can support
// different configurations. This approach allows for different ways to run functions with
// without specifying the configuration repeatedly.
func (o *Observer) UseRunConfig(config RunConfig) *Observer {
	newObserver := *o
	newObserver.runner = config
	return &newObserver
}

// TODO: document.
// Used to set whether panic recovery should be disabled. Recovery is enabled by default.
// It is recommended against disabling panic recovery unless you have a reason to propagate
// panics to the caller. An alternative means is to retrieve panic value from the error using
// [IsPanicError] after Run* methods have been executed.
func (o *Observer) DisableRecovery() *Observer {
	newObserver := *o
	newObserver.recoverPanics = false
	return &newObserver
}

// Run executes fn and records metrics according to the observer's configuration.
// If a timeout is specified by the observer, it will not be respected by this method.
// Note if [context.DeadlineExceeded] is returned by fn, it can be recorded as a timeout.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	observer.Run(func() error {
//		return nil
//	})
func (o *Observer) Run(fn func() error) error {
	if o == nil || o.metrics == nil {
		panic("observer: not configured")
	}

	task := &implTask{
		fn: func(ctx context.Context) error {
			return fn() // ignore ctx
		},
	}
	if o.runner.Control != nil {
		if o.runner.Control() {
			return &ErrControlBreaker{}
		}
	}

	return o.observe(task)
}

// RunFunc executes fn and records metrics according to the observer's configuration.
// For timeouts specified by the observer, the fn will be passed a context with the timeout.
// Note if [context.DeadlineExceeded] is returned by fn, it can be recorded as a timeout.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	observer.RunFunc(func(ctx context.Context) error {
//		return nil
//	})
func (o *Observer) RunFunc(fn func(ctx context.Context) error) error {
	if o == nil || o.metrics == nil {
		panic("observer: not configured")
	}

	task := &implTask{
		fn: fn,
	}
	if o.runner.Control != nil {
		if o.runner.Control() {
			return &ErrControlBreaker{}
		}
	}

	return o.observe(task)
}

// observe is the main entry point for observing a task.
func (o *Observer) observe(task *implTask) (err error) {
	o.metrics.InFlight.Inc()
	defer o.metrics.InFlight.Dec()
	return o.execute(task)
}

// execute is the main entry point for executing a task.
func (o *Observer) execute(task *implTask) error {
	var ctx = context.Background()
	if o.runner.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.runner.Timeout)
		defer cancel()
	}

	// Run task in closure
	var panicValue any
	err := func() (err error) {
		start := time.Now()
		defer func() {
			if o.metrics.Durations != nil {
				o.metrics.Durations.Observe(time.Since(start).Seconds())
			}
			if r := recover(); r != nil {
				panicValue = r
				err = &ErrRecoveredPanic{panic: r}
			}
		}()
		err = task.fn(ctx)
		return err
	}()

	// Handle errors
	if err != nil {
		o.metrics.Errors.Inc()
		if o.metrics.Timeouts != nil && errors.Is(err, context.DeadlineExceeded) {
			o.metrics.Timeouts.Inc()
		}

		// Handle panics
		if panicValue != nil {
			o.metrics.Panics.Inc()
			if !o.recoverPanics {
				panic(panicValue) // re-throw
			}
		}

		// Handle retries
		if o.runner.MaxRetries > 0 {

			// Maximum retries reached
			if task.retryCount >= o.runner.MaxRetries {
				o.metrics.Failures.Inc()
				return err
			}

			// Try circuit breakers
			if o.runner.RetryBreaker != nil {
				if o.runner.RetryBreaker(err) {
					o.metrics.Failures.Inc()
					return err
				}
			}
			if o.runner.Control != nil {
				if o.runner.Control() {
					o.metrics.Failures.Inc()
					return err
				}
			}

			// Wait retry duration
			if o.metrics.Retries != nil {
				o.metrics.Retries.Inc()
			}
			if o.runner.RetryStrategy != nil {
				wait := o.runner.RetryStrategy(task.retryCount)
				if wait > 0 {
					time.Sleep(wait)
				}
			}

			// Next retry attempt
			retryTask := &implTask{
				fn:         task.fn,
				retryCount: task.retryCount + 1,
			}

			// Try retries recursively
			err2 := o.observe(retryTask)
			if err2 != nil {
				return errors.Join(err, err2)
			} else {
				return nil
			}
		} else {
			o.metrics.Failures.Inc()
		}
	} else {
		o.metrics.Successes.Inc()
	}

	return err
}
