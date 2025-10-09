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

// executeTask performs the task execution logic and handles retries.
// Retry attempts are call method recursively for synchronous task handling.
// If panic occurs, the error from task.Execute() is switched for ErrRecoveredPanic.
// This follows the behaviour defined by the [Task], [TaskConfig], and [Observer].

// Observer monitors and measures task executions, collecting Prometheus metrics
// for successes, failures, timeouts, panics, retries, and observed runtimes.
// It provides methods to execute tasks with various TaskConfig options.
type Observer struct {
	cfg     observerConfig
	metrics *metrics
}

// NewObserver creates a new Observer instance with the specified options.
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
//	// With fully qualified name and records runtime durations
//	observer := sentinel.NewObserver(
//	  sentinel.WithNamespace("myapp"),
//	  sentinel.WithSubsystem("workers"),
//	  sentinel.WithHistogramBuckets([]float64{0.05, 1, 5, 30, 600}),
//	)
func NewObserver(opts ...ObserverOption) *Observer {
	cfg := observerConfig{
		namespace:     "",
		subsystem:     "",
		description:   "tasks",
		recoverPanics: true,
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
	if cfg.taskConfig == nil {
		cfg.taskConfig = &TaskConfig{}
	}

	return &Observer{
		cfg:     cfg,
		metrics: newMetrics(cfg),
	}
}

// Register registers all Observer metrics with the provided Prometheus registry.
// Use [Observer.MustRegister] if you want the program to panic on registration conflicts.
func (o *Observer) Register(registry prometheus.Registerer) error {
	return o.metrics.Register(registry)
}

// MustRegister registers all Observer metrics with the provided Prometheus registry.
// This method panics if any metric registration failures. Use [Observer.Register] if you prefer
// to handle registration errors gracefully.
func (o *Observer) MustRegister(registry prometheus.Registerer) {
	o.metrics.MustRegister(registry)
}

// Run executes a function with the specified task configuration and observes its execution.
// Any timeout specified through the task configuration will be ignored by the function.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	observer.Run(sentinel.TaskConfig{}, func() error {
//		return nil
//	})
func (o *Observer) Run(cfg TaskConfig, fn func() error) error {
	if o == nil || o.metrics == nil {
		panic("observer: not configured")
	}
	task := &implTask{
		cfg: cfg,
		fn: func(ctx context.Context) error {
			return fn() // ignore ctx
		},
	}
	return o.observe(task)
}

// Run executes a function with the specified task configuration and observes its execution.
// Any timeout specified through the task configuration will be set by the context passed to fn.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	observer.RunCtx(sentinel.TaskConfig{}, func(ctx context.Context) error {
//		return nil
//	})
func (o *Observer) RunCtx(cfg TaskConfig, fn func(ctx context.Context) error) error {
	if o == nil || o.metrics == nil {
		panic("observer: not configured")
	}
	task := &implTask{
		cfg: cfg,
		fn:  fn,
	}
	return o.observe(task)
}

// RunFunc executes a function with the DefaultTaskConfig and observes its execution.
// Any timeout specified through the task configuration will be ignored by the function.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	observer.RunFunc(func() error {
//		return nil
//	})
func (o *Observer) RunFunc(fn func() error) error {
	if o == nil || o.metrics == nil {
		panic("observer: not configured")
	}
	task := &implTask{
		cfg: *o.cfg.taskConfig,
		fn: func(ctx context.Context) error {
			return fn() // ignore ctx
		},
	}
	return o.observe(task)
}

// RunFuncCtx executes a function with the DefaultTaskConfig and observes its execution.
// Any timeout specified through the task configuration will be set by the context passed to fn.
//
// Example usage:
//
//	observer := sentinel.NewObserver()
//	observer.RunFuncCtx(func(ctx context.Context) error {
//		return nil
//	})
func (o *Observer) RunFuncCtx(fn func(ctx context.Context) error) error {
	if o == nil || o.metrics == nil {
		panic("observer: not configured")
	}
	task := &implTask{
		cfg: *o.cfg.taskConfig,
		fn:  fn,
	}
	return o.observe(task)
}

// RunTask executes a [Task] implementation and observes its execution.
// Any timeout specified through the task configuration will be set by the context passed to task.Execute.
//
// Example usage:
//
//	type CustomTask struct {}
//	func (t *CustomTask) Config() sentinel.TaskConfig { /* ... */ }
//	func (t *CustomTask) Execute(ctx context.Context) error { /* ... */ }
//
//	observer := sentinel.NewObserver()
//	observer.RunTask(&CustomTask{})
func (o *Observer) RunTask(task Task) error {
	if o == nil || o.metrics == nil {
		panic("observer: not configured")
	}
	t := &implTask{
		cfg: task.Config(),
		fn:  task.Execute,
	}
	return o.observe(t)
}

func (o *Observer) observe(task *implTask) (err error) {
	o.metrics.InFlight.Inc()
	defer o.metrics.InFlight.Dec()
	return o.executeTask(task)
}

func (o *Observer) observeRuntime(start time.Time) {
	if o.metrics.ObservedRuntimes != nil {
		o.metrics.ObservedRuntimes.Observe(time.Since(start).Seconds())
	}
}

func (o *Observer) executeTask(task *implTask) error {
	var ctx = context.Background()
	if task.Config().Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.Config().Timeout)
		defer cancel()
	}

	// Run task in closure to capture the panic
	var panicValue any
	err := func() (err error) {
		start := time.Now()
		defer func() {
			o.observeRuntime(start)
			if r := recover(); r != nil {
				panicValue = r
				err = &ErrRecoveredPanic{panic: r}
			}
		}()
		err = task.Execute(ctx)
		return err
	}()

	// Handle errors
	if err != nil {
		o.metrics.Errors.Inc()
		if errors.Is(err, context.DeadlineExceeded) {
			o.metrics.TimeoutErrors.Inc()
		}

		// Handle panics
		if panicValue != nil {
			o.metrics.Panics.Inc()
			if !o.cfg.recoverPanics {
				panic(panicValue) // re-throw panic
			}
		}

		// Handle retries
		if task.Config().MaxRetries > 0 {
			cfg := task.Config()

			// Maximum retries reached
			if task.retryCount >= cfg.MaxRetries {
				return err
			}

			// Try circuit break
			if cfg.CircuitBreaker != nil {
				if cfg.CircuitBreaker(err) {
					return err
				}
			}

			// Wait retry duration
			o.metrics.Retries.Inc()
			if cfg.RetryStrategy != nil {
				wait := cfg.RetryStrategy(task.retryCount)
				if wait > 0 {
					time.Sleep(wait)
				}
			}

			// Next retry attempt
			retryTask := &implTask{
				fn:         task.Execute,
				cfg:        cfg,
				retryCount: task.retryCount + 1,
			}

			// Run retry task
			err2 := o.observe(retryTask)
			if err2 != nil {
				return errors.Join(err, err2)
			} else {
				return nil // successful recursive return
			}
		}
	} else {
		o.metrics.Successes.Inc()
	}

	return err
}
