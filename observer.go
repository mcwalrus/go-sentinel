// Package sentinel provides reliability handling and observability monitoring for Go applications.
// It wraps task execution with Prometheus metrics, observing errors, panic occurrences, retries,
// and timeouts — making critical routines safe, measurable, and reliable.
package sentinel

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Observer monitors and measures task executions, collecting Prometheus metrics
// for successes, failures, timeouts, panics, retries, and observed runtimes.
// It provides methods to execute tasks with various TaskConfig options.
type Observer struct {
	m             *sync.RWMutex
	cfg           observerConfig
	metrics       *metrics
	recoverPanics bool
	runner        TaskConfig
}

// NewObserver configures a new Observer with specified [PrometheusOption] options.
// The Observer will need to be registered with a Prometheus registry to expose metrics.
// Please refer to [Observer.MustRegister] and [Observer.Register] for more information.
//
// Example usage:
//
//	// Default configuration
//	observer := sentinel.NewObserver()
//
//	// Metrics with new namespace and subsystem
//	observer := sentinel.NewObserver(
//	  sentinel.WithNamespace("my_app"),
//	  sentinel.WithSubsystem("workers"),
//	)
//
//	// Support metrics variants
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
		m:       &sync.RWMutex{},
		cfg:     cfg,
		metrics: newMetrics(cfg),
	}
}

// TODO: redo / revise docs.
// Register registers all Observer metrics with the provided Prometheus registry.
// Use [Observer.MustRegister] if you want the program to panic on registration conflicts.
func (o *Observer) Register(registry prometheus.Registerer) error {
	return o.metrics.Register(registry)
}

// TODO: redo / revise docs.
// MustRegister registers all Observer metrics with the provided Prometheus registry.
// This method panics if any metric registration failures. Use [Observer.Register] if you prefer
// to handle registration errors gracefully.
func (o *Observer) MustRegister(registry prometheus.Registerer) {
	o.metrics.MustRegister(registry)
}

// TODO: redo docs.
// Fork creates a new Observer with shared underlying metrics as the current Observer.
// The child shares the underlying metrics but hold different configurations for how to handle
// Run methods including the use of different TaskConfig and recovery behaviour.
func (o *Observer) Fork() *Observer {
	newObserver := *o
	newObserver.m = &sync.RWMutex{}
	return &newObserver
}

// TODO: redo docs.
// UseConfig configures the observer for how to handle Run methods.
// See [TaskConfig] for more information on how to configure the Run method behaviour.
func (o *Observer) UseConfig(config TaskConfig) {
	o.m.Lock()
	o.runner = config
	o.m.Unlock()
}

// TODO: revise / improve docs.
// Used to set whether panic recovery should be disabled. Recovery is enabled by default.
// It is recommended against disabling panic recovery unless you have a reason to propagate
// panics to the caller. An alternative means is to retrieve panic value from the error using
// [IsPanicError] after Run* methods have been executed.
func (o *Observer) DisablePanicRecovery(disable bool) {
	o.m.Lock()
	o.recoverPanics = !disable
	o.m.Unlock()
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
	o.m.RLock()
	cfg := o.runner
	o.m.RUnlock()

	task := &implTask{
		cfg: cfg,
		fn: func(ctx context.Context) error {
			return fn() // ignore ctx
		},
	}
	if cfg.Control != nil {
		if cfg.Control() {
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
	o.m.RLock()
	cfg := o.runner
	o.m.RUnlock()

	task := &implTask{
		cfg: cfg,
		fn:  fn,
	}
	if cfg.Control != nil {
		if cfg.Control() {
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

	// Respect timeout
	if task.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.cfg.Timeout)
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
			o.m.RLock()
			if !o.recoverPanics {
				o.m.RUnlock()
				panic(panicValue) // re-throw
			}
			o.m.RUnlock()
		}

		// Handle retries
		if task.cfg.MaxRetries > 0 {

			// Maximum retries reached
			if task.retryCount >= task.cfg.MaxRetries {
				o.metrics.Failures.Inc()
				return err
			}

			// Try circuit breakers
			if task.cfg.RetryBreaker != nil {
				if task.cfg.RetryBreaker(err) {
					o.metrics.Failures.Inc()
					return err
				}
			}
			if task.cfg.Control != nil {
				if task.cfg.Control() {
					o.metrics.Failures.Inc()
					return err
				}
			}

			// Wait retry duration
			if o.metrics.Retries != nil {
				o.metrics.Retries.Inc()
			}
			if task.cfg.RetryStrategy != nil {
				wait := task.cfg.RetryStrategy(task.retryCount)
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
