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
	metrics       metrics
	runner        ObserverConfig
	controls      ObserverControls
	labelValues   []string
	recoverPanics bool
	parent        *VecObserver
}

// NewObserver configures a new Observer with duration buckets and optional configuration.
// The Observer will need to be registered with a Prometheus registry to expose metrics.
// Please refer to [Observer.MustRegister] and [Observer.Register] for more information.
//
// Example usage:
//
//	// Default configuration
//	observer := sentinel.NewObserver(nil)
//
//	// With duration metrics
//	observer := sentinel.NewObserver([]float64{0.05, 1, 5, 30, 600})
//
//	// With custom namespace and subsystem
//	observer := sentinel.NewObserver(
//	  []float64{0.1, 0.5, 1, 2, 5},
//	  sentinel.WithNamespace("my_app"),
//	  sentinel.WithSubsystem("workers"),
//	)
func NewObserver(durationBuckets []float64, opts ...ObserverOption) *Observer {
	cfg := setupConfig(durationBuckets, opts...)
	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           cfg,
		metrics:       newMetrics(cfg),
		recoverPanics: true,
	}
}

func (o *Observer) Describe(ch chan<- *prometheus.Desc) {
	o.metrics.Describe(ch)
}

func (o *Observer) Collect(ch chan<- prometheus.Metric) {
	o.metrics.Collect(ch)
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

// UseConfig configures the observer for how to handle Run methods.
// This sets the ObserverConfig that will be used for all subsequent Run, RunFunc calls.
// See [ObserverConfig] for more information on available configuration options.
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
	o.controls = config.Controls
	o.m.Unlock()
}

// DisablePanicRecovery sets whether panic recovery should be disabled for the observer.
// Recovery is enabled by default, meaning panics are caught and converted to errors.
func (o *Observer) DisablePanicRecovery(disable bool) {
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
	if o == nil {
		panic("observer: not configured")
	}
	o.m.RLock()
	cfg := o.runner
	controls := o.controls
	o.m.RUnlock()

	task := &implTask{
		cfg: cfg,
		fn: func(ctx context.Context) error {
			return fn() // ignore ctx
		},
	}

	if controls.RequestControl != nil {
		if controls.RequestControl() {
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
	if o == nil {
		panic("observer: not configured")
	}
	o.m.RLock()
	cfg := o.runner
	controls := o.controls
	o.m.RUnlock()

	task := &implTask{
		cfg: cfg,
		fn:  fn,
	}
	if controls.RequestControl != nil {
		if controls.RequestControl() {
			return &ErrControlBreaker{}
		}
	}

	return o.observe(task)
}

// observe is the main entry point for observing a task.
func (o *Observer) observe(task *implTask) (err error) {
	o.metrics.inFlight.Inc()
	defer o.metrics.inFlight.Dec()
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
			if o.metrics.durations != nil {
				o.metrics.durations.Observe(time.Since(start).Seconds())
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
		o.metrics.errors.Inc()
		if errors.Is(err, context.DeadlineExceeded) {
			o.metrics.timeouts.Inc()
		}

		// Handle panics
		if panicValue != nil {
			o.metrics.panics.Inc()
			o.m.RLock()
			if !o.recoverPanics {
				o.m.RUnlock()
				o.metrics.failures.Inc()
				panic(panicValue) // re-throw
			}
			o.m.RUnlock()
		}

		// Handle retries
		if task.cfg.MaxRetries > 0 {

			// Maximum retries reached
			if task.retryCount >= task.cfg.MaxRetries {
				o.metrics.failures.Inc()
				return err
			}

			// Try circuit breaker
			if task.cfg.RetryBreaker != nil {
				if task.cfg.RetryBreaker(err) {
					o.metrics.failures.Inc()
					return err
				}
			}
			// Try in-flight control
			o.m.RLock()
			if o.controls.InFlightControl != nil {
				if o.controls.InFlightControl() {
					o.m.RUnlock()
					o.metrics.failures.Inc()
					return err
				}
			}
			o.m.RUnlock()

			// Wait retry duration
			task.retryCount += 1
			o.metrics.retries.Inc()
			if task.cfg.RetryStrategy != nil {
				wait := task.cfg.RetryStrategy(task.retryCount)
				if wait > 0 {
					time.Sleep(wait)
				}
			}

			// Next retry attempt
			retryTask := &implTask{
				fn:         task.fn,
				cfg:        task.cfg,
				retryCount: task.retryCount,
			}

			// Try retries recursively
			err2 := o.observe(retryTask)
			if err2 != nil {
				return errors.Join(err, err2)
			} else {
				return nil
			}
		} else {
			o.metrics.failures.Inc()
		}
	} else {
		o.metrics.successes.Inc()
	}

	return err
}
