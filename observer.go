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
// It provides methods to execute tasks with various ObserverConfig options.
type Observer struct {
	m             *sync.RWMutex
	cfg           config
	metrics       *metrics
	recoverPanics bool
	runner        ObserverConfig
}

// NewObserver configures a new Observer with specified [ObserverOption] options.
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
func NewObserver(opts ...ObserverOption) *Observer {
	cfg := config{
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

// Register registers all Observer metrics with the provided Prometheus registry.
// This method returns an error if any metric registration fails. Use [Observer.MustRegister]
// if you want the program to panic on registration conflicts instead of handling errors.
//
// Example usage:
//
//	registry := prometheus.NewRegistry()
//	if err := observer.Register(registry); err != nil {
//		log.Fatalf("Failed to register metrics: %v", err)
//	}
func (o *Observer) Register(registry prometheus.Registerer) error {
	return o.metrics.Register(registry)
}

// MustRegister registers all Observer metrics with the provided Prometheus registry.
// This method panics if any metric registration failures occur. Use [Observer.Register]
// if you prefer to handle registration errors gracefully instead of panicking.
//
// Example usage:
//
//	registry := prometheus.NewRegistry()
//	observer.MustRegister(registry) // Will panic if registration fails
func (o *Observer) MustRegister(registry prometheus.Registerer) {
	o.metrics.MustRegister(registry)
}

// Fork creates a new Observer with shared underlying metrics as the current Observer.
//
// This is useful when you want multiple observers to share the same metric namespace
// but have different execution behaviors (e.g., different retry strategies, timeouts).
//
// Example usage:
//
//	baseObserver := sentinel.NewObserver(sentinel.WithNamespace("myapp"))
//	criticalObserver := baseObserver.Fork()
//	criticalObserver.UseConfig(sentinel.ObserverConfig{
//		MaxRetries: 5,
//		Timeout:    30 * time.Second,
//	})
func (o *Observer) Fork() *Observer {
	newObserver := *o
	newObserver.m = &sync.RWMutex{}
	return &newObserver
}

// UseConfig configures the observer for how to handle Run methods.
// This sets the ObserverConfig that will be used for all subsequent Run, RunFunc calls.
// See [ObserverConfig] for more information on available configuration options.
//
// The configuration includes settings for timeouts, retry strategies, circuit breakers,
// and other execution behaviors. This method is thread-safe.
//
// Example usage:
//
//	observer.UseConfig(sentinel.ObserverConfig{
//		Timeout:    10 * time.Second,
//		MaxRetries: 3,
//		RetryStrategy: retry.Exponential(100 * time.Millisecond),
//	})
func (o *Observer) UseConfig(config ObserverConfig) {
	o.m.Lock()
	o.runner = config
	o.m.Unlock()
}

// DisableRecovery sets whether panic recovery should be disabled for the observer.
// Recovery is enabled by default, meaning panics are caught and converted to errors.
func (o *Observer) DisableRecovery(disable bool) {
	o.m.Lock()
	o.recoverPanics = !disable
	o.m.Unlock()
}

// Run executes fn and records metrics according to the observer's configuration.
// This method does not respect timeouts set in the observer's ObserverConfig.
// Use RunFunc if you need timeout support.
//
// The function is executed with panic recovery enabled by default. Panics are
// converted to errors and recorded in metrics. Use DisablePanicRecovery(true)
// to propagate panics instead.
//
// If the function returns context.DeadlineExceeded, it will be recorded as a timeout
// when timeout metrics are enabled.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	err := observer.Run(func() error {
//		return nil
//	})
//	if err != nil {
//		log.Printf("Task failed: %v", err)
//	}
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
// For timeouts specified by the observer's ObserverConfig, the fn will be passed a context
// with the timeout. This is the recommended method when you need timeout support.
//
// The function is executed with panic recovery enabled by default. Panics are
// converted to errors and recorded in metrics. Use DisablePanicRecovery(true)
// to propagate panics instead.
//
// If the function returns context.DeadlineExceeded, it will be recorded as a timeout
// when timeout metrics are enabled.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	observer.UseConfig(sentinel.ObserverConfig{
//		Timeout: 10 * time.Second,
//	})
//	err := observer.RunFunc(func(ctx context.Context) error {
//		// Your task logic here with timeout support
//		select {
//		case <-ctx.Done():
//			return ctx.Err()
//		case <-time.After(5 * time.Second):
//			return nil
//		}
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
