// Package sentinel provides reliability handling and observability monitoring for Go applications.
// It wraps task execution with Prometheus metrics, observing errors, panic occurrences, retries,
// and timeouts — making critical routines safe, measurable, and reliable.
package sentinel

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mcwalrus/go-sentinel/circuit"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sourcegraph/conc/pool"
)

// retryCountKey is the context key for storing the retry count.
type retryCountKey struct{}

// RetryCount returns the current retry count from the context.
// Returns 0 if the retry count is not set in the context.
//
// Example usage:
//
//	observer := sentinel.NewObserver(sentinel.WithRetrier(retry.DefaultRetrier{MaxRetries: 3}))
//	_ = observer.RunFunc(func(ctx context.Context) error {
//		retryCount := sentinel.RetryCount(ctx)
//		log.Printf("Current retry count: %d\n", retryCount)
//		return nil
//	})
func RetryCount(ctx context.Context) int {
	if count, ok := ctx.Value(retryCountKey{}).(int); ok {
		return count
	}
	return 0
}

// Observer wraps a function and automatically records Prometheus metrics for each
// execution. It tracks successes, failures, timeouts, panics, retries, and durations.
// Create with [NewObserver] or [NewObserverDefault].
type Observer struct {
	m             *sync.RWMutex
	cfg           config
	metrics       metrics
	control       circuit.Control
	limiter       limiter
	pool          *pool.Pool
	labelValues   []string
	recoverPanics bool

	poolErrsMu sync.Mutex
	poolErrs   []error
}

// NewObserver returns a new Observer configured by the provided options.
// opts are applied in order; later options override earlier ones where they conflict.
// The returned Observer must be registered with a Prometheus registry before metrics
// are exposed. See [Observer.Register] and [Observer.MustRegister].
//
// Example usage:
//
//	// Default configuration
//	observer := sentinel.NewObserver()
//
//	// With duration metrics
//	observer := sentinel.NewObserver(sentinel.WithDurationMetrics([]float64{0.05, 1, 5, 30, 600}))
//
//	// With custom namespace and subsystem
//	observer := sentinel.NewObserver(
//	  sentinel.WithDurationMetrics([]float64{0.1, 0.5, 1, 2, 5}),
//	  sentinel.WithNamespace("my_app"),
//	  sentinel.WithSubsystem("workers"),
//	)
func NewObserver(opts ...ObserverOption) *Observer {
	cfg := setupConfig(opts...)
	p := pool.New()
	if cfg.maxConcurrency > 0 {
		p = p.WithMaxGoroutines(cfg.maxConcurrency)
	}
	obs := &Observer{
		m:             &sync.RWMutex{},
		cfg:           cfg,
		metrics:       newMetrics(cfg),
		pool:          p,
		recoverPanics: true,
	}
	if cfg.control != nil {
		obs.control = cfg.control
	}
	if cfg.maxConcurrency > 0 {
		obs.limiter = make(limiter, cfg.maxConcurrency)
	}
	return obs
}

// NewObserverDefault returns an Observer with in-flight, success, and error metrics
// enabled by default. Additional opts are appended after the defaults, allowing callers
// to override namespace, subsystem, or add extra metrics. Use [NewObserver] for full
// control over which metrics are exported.
//
// Example usage:
//
//	// Default metrics: in_flight, success_total, errors_total, failures_total
//	observer := sentinel.NewObserverDefault()
//
//	// Extend defaults with additional options
//	observer := sentinel.NewObserverDefault(
//	    sentinel.WithNamespace("my_app"),
//	    sentinel.WithSubsystem("workers"),
//	)
func NewObserverDefault(opts ...ObserverOption) *Observer {
	defaultOpts := []ObserverOption{
		WithInFlightMetrics(),
		WithSuccessMetrics(),
		WithErrorMetrics(),
	}
	return NewObserver(append(defaultOpts, opts...)...) //nolint:gocritic
}

// Describe implements the [prometheus.Collector] interface by describing metrics.
// This can be useful to register the Observer with the default Prometheus registry.
func (o *Observer) Describe(ch chan<- *prometheus.Desc) {
	o.metrics.Describe(ch)
}

// Collect implements the [prometheus.Collector] interface by collecting metrics.
// This can be useful to register the Observer with the default Prometheus registry.
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

// DisablePanicRecovery sets whether panic recovery should be disabled for the observer.
// Recovery is enabled by default, meaning panics are caught and converted to errors.
// When disabled, panics will propagate normally and may crash the program.
//
// Note: for async tasks submitted via [Observer.Submit] or [Observer.SubmitFunc],
// disabling panic recovery causes the panic to escape the goroutine and be captured
// by the underlying conc pool. The pool will re-panic when [pool.Wait] is called,
// rather than crashing immediately. This differs from the synchronous path where
// the panic propagates directly to the caller of Run or RunFunc.
func (o *Observer) DisablePanicRecovery(disable bool) {
	o.m.Lock()
	o.recoverPanics = !disable
	o.m.Unlock()
}

// Run executes fn synchronously and records metrics according to the observer's
// configuration. The call blocks until fn returns.
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
	control := o.control
	limiter := o.limiter
	o.m.RUnlock()

	task := &implTask{
		fn: func(_ context.Context) error {
			return fn() // ignore ctx
		},
	}

	return o.observe(limiter, control, task)
}

// RunFunc executes fn synchronously and records metrics according to the observer's
// configuration. The call blocks until fn returns.
// For timeouts configured via [WithTimeout], fn will be passed a context with the
// timeout applied. This is the recommended method when you need timeout support.
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
//	observer := sentinel.NewObserver(sentinel.WithTimeout(10 * time.Second))
//
//	err := observer.RunFunc(func(ctx context.Context) error {
//		// Your task logic here with timeout support
//		select {
//		case <-ctx.Done():
//			return ctx.Err()
//		case <-time.After(5 * time.Second):
//			return nil
//		}
//	})
//	if err != nil {
//		log.Printf("Task failed: %v", err)
//	}
func (o *Observer) RunFunc(fn func(ctx context.Context) error) error {
	if o == nil {
		panic("observer: not configured")
	}
	o.m.RLock()
	control := o.control
	limiter := o.limiter
	o.m.RUnlock()

	task := &implTask{
		fn: fn,
	}

	return o.observe(limiter, control, task)
}

// Submit enqueues fn in the Observer's worker pool for async execution.
// All metrics (in_flight, success/error counters, durations) are recorded
// when fn executes. The method returns immediately without waiting for fn
// to complete. Panics in fn are captured by the pool and surface via Wait().
//
// Callers must call [Observer.Wait] to drain the pool and collect errors.
// Failing to call Wait may leave goroutines running and errors unobserved.
//
// Example usage:
//
//	observer := sentinel.NewObserver(nil)
//	observer.Submit(func() error {
//		return doWork()
//	})
//	if err := observer.Wait(); err != nil {
//		log.Printf("tasks failed: %v", err)
//	}
func (o *Observer) Submit(fn func() error) {
	if o == nil {
		panic("observer: not configured")
	}
	o.m.RLock()
	control := o.control
	limiter := o.limiter
	o.m.RUnlock()

	task := &implTask{
		fn: func(_ context.Context) error {
			return fn()
		},
	}

	if o.cfg.enablePending && o.cfg.maxConcurrency > 0 && o.metrics.pending != nil {
		o.metrics.pending.Inc()
		o.pool.Go(func() {
			o.metrics.pending.Dec()
			if err := o.observe(limiter, control, task); err != nil {
				o.poolErrsMu.Lock()
				o.poolErrs = append(o.poolErrs, err)
				o.poolErrsMu.Unlock()
			}
		})
	} else {
		o.pool.Go(func() {
			if err := o.observe(limiter, control, task); err != nil {
				o.poolErrsMu.Lock()
				o.poolErrs = append(o.poolErrs, err)
				o.poolErrsMu.Unlock()
			}
		})
	}
}

// SubmitFunc enqueues fn in the Observer's worker pool for async execution.
// fn receives a context with a timeout applied if WithTimeout was configured.
// All metrics (in_flight, success/error counters, durations) are recorded
// when fn executes. The method returns immediately without waiting for fn
// to complete. Panics in fn are captured by the pool and surface via Wait().
//
// Callers must call [Observer.Wait] to drain the pool and collect errors.
// Failing to call Wait may leave goroutines running and errors unobserved.
//
// Example usage:
//
//	observer := sentinel.NewObserver(sentinel.WithTimeout(5 * time.Second))
//	observer.SubmitFunc(func(ctx context.Context) error {
//		select {
//		case <-ctx.Done():
//			return ctx.Err()
//		default:
//			return doWork()
//		}
//	})
//	if err := observer.Wait(); err != nil {
//		log.Printf("tasks failed: %v", err)
//	}
func (o *Observer) SubmitFunc(fn func(ctx context.Context) error) {
	if o == nil {
		panic("observer: not configured")
	}
	o.m.RLock()
	control := o.control
	limiter := o.limiter
	o.m.RUnlock()

	task := &implTask{
		fn: fn,
	}

	if o.cfg.enablePending && o.cfg.maxConcurrency > 0 && o.metrics.pending != nil {
		o.metrics.pending.Inc()
		o.pool.Go(func() {
			o.metrics.pending.Dec()
			if err := o.observe(limiter, control, task); err != nil {
				o.poolErrsMu.Lock()
				o.poolErrs = append(o.poolErrs, err)
				o.poolErrsMu.Unlock()
			}
		})
	} else {
		o.pool.Go(func() {
			if err := o.observe(limiter, control, task); err != nil {
				o.poolErrsMu.Lock()
				o.poolErrs = append(o.poolErrs, err)
				o.poolErrsMu.Unlock()
			}
		})
	}
}

// Wait blocks until all submitted tasks complete and returns any errors.
// The observer is reusable after Wait returns.
//
// Errors from all submitted tasks are collected and returned as a joined error.
// If no tasks were submitted, or all tasks succeeded, Wait returns nil.
//
// Example usage:
//
//	observer := sentinel.NewObserver(nil)
//	observer.Submit(func() error { return doWork() })
//	observer.Submit(func() error { return doMoreWork() })
//	if err := observer.Wait(); err != nil {
//		log.Printf("tasks failed: %v", err)
//	}
//	// observer is now reusable
//	observer.Submit(func() error { return doNextBatch() })
func (o *Observer) Wait() error {
	if o == nil {
		panic("observer: not configured")
	}
	o.pool.Wait()

	// Collect errors and reset for next batch
	o.poolErrsMu.Lock()
	errs := o.poolErrs
	o.poolErrs = nil
	o.poolErrsMu.Unlock()

	// Reset pool so it is reusable
	o.m.Lock()
	p := pool.New()
	if o.cfg.maxConcurrency > 0 {
		p = p.WithMaxGoroutines(o.cfg.maxConcurrency)
	}
	o.pool = p
	o.m.Unlock()

	return errors.Join(errs...)
}

// observe is the main entry point for observing a task.
// It acquires the limiter slot if set and checks control for new request phase
// before executing the task. If the control returns true, it returns an error.
func (o *Observer) observe(limiter limiter, control circuit.Control, task *implTask) error {
	var releaseLimiter func()

	// Acquire limiter slot if set
	if limiter != nil {
		acquired := limiter.acquire()
		select {
		case <-acquired:
		default:
			if o.metrics.pending != nil {
				o.metrics.pending.Inc()
			}
			<-acquired
			if o.metrics.pending != nil {
				o.metrics.pending.Dec()
			}
		}
		releaseLimiter = func() { limiter.release() }
	}

	// Check control on new request phase
	if control != nil {
		shouldStop, panicked := safeControl(control, circuit.PhaseNewRequest)
		if panicked && o.metrics.panics != nil {
			o.metrics.panics.Inc()
		}
		if shouldStop {
			if releaseLimiter != nil {
				releaseLimiter()
			}
			if o.metrics.errors != nil {
				o.metrics.errors.Inc()
			}
			if o.metrics.failures != nil {
				o.metrics.failures.Inc()
			}
			return &ErrControlBreaker{}
		}
	}

	if releaseLimiter != nil {
		defer releaseLimiter()
	}

	if o.metrics.inFlight != nil {
		o.metrics.inFlight.Inc()
		defer o.metrics.inFlight.Dec()
	}

	return o.execute(task)
}

// filterControlBreaker removes ErrControlBreaker from a (potentially joined) error.
// It returns nil if all constituent errors are ErrControlBreaker instances, or the
// remaining non-control-breaker errors joined together.
func filterControlBreaker(err error) error {
	if err == nil {
		return nil
	}
	// Try to unwrap a joined error (errors.Join returns an interface with Unwrap() []error).
	type joinedErrors interface {
		Unwrap() []error
	}
	if je, ok := err.(joinedErrors); ok {
		var remaining []error
		for _, e := range je.Unwrap() {
			if !errors.As(e, new(*ErrControlBreaker)) {
				remaining = append(remaining, e)
			}
		}
		return errors.Join(remaining...)
	}
	// Single error: return nil if it is ErrControlBreaker, otherwise return as-is.
	if errors.As(err, new(*ErrControlBreaker)) {
		return nil
	}
	return err
}


// safeControl calls the control handler with panic recovery.
// If the handler panics, it returns false (allow execution) as a safe default,
// and signals that a panic occurred via the second return value.
func safeControl(control circuit.Control, phase circuit.ExecutionPhase) (shouldStop bool, panicked bool) {
	if control == nil {
		return false, false
	}
	defer func() {
		if r := recover(); r != nil {
			shouldStop = false
			panicked = true
			_ = r
		}
	}()
	return control(phase), false
}

// safeErrorLabeler calls the error labeler with panic recovery.
// If the labeler panics, it returns nil labels and signals that a panic occurred.
func safeErrorLabeler(labeler func(err error) prometheus.Labels, err error) (labels prometheus.Labels, panicked bool) {
	if labeler == nil {
		return nil, false
	}
	defer func() {
		if r := recover(); r != nil {
			labels = nil
			panicked = true
			_ = r
		}
	}()
	return labeler(err), false
}

// execute is the main entry point for executing a task.
func (o *Observer) execute(task *implTask) error {
	var ctx = context.Background()

	if o.cfg.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.cfg.timeout)
		defer cancel()
	}

	// If an observer-level Retrier is configured, delegate all retry orchestration to it.
	// The Retrier calls the wrapped fn for each attempt (initial + retries).
	if o.cfg.retrier != nil {
		attempt := 0
		controlStopped := false
		err := o.cfg.retrier.Do(ctx, func() error {
			currentAttempt := attempt
			attempt++

			// If a previous iteration set the control-stop flag, propagate the
			// control breaker error so the retrier's joined result reflects it.
			if controlStopped {
				return &ErrControlBreaker{}
			}

			if currentAttempt > 0 {
				// Retry attempt: check control gate before proceeding.
				o.m.RLock()
				if o.control != nil {
					shouldStop, panicked := safeControl(o.control, circuit.PhaseRetry)
					if panicked && o.metrics.panics != nil {
						o.metrics.panics.Inc()
					}
					if shouldStop {
						o.m.RUnlock()
						controlStopped = true
						return &ErrControlBreaker{}
					}
				}
				o.m.RUnlock()
				if o.metrics.retries != nil {
					o.metrics.retries.Inc()
				}
			}

			retryCtx := context.WithValue(ctx, retryCountKey{}, currentAttempt)

			var panicValue any
			fnErr := func() (err error) {
				start := time.Now()
				defer func() {
					if o.metrics.durations != nil {
						o.metrics.durations.Observe(time.Since(start).Seconds())
					}
					if r := recover(); r != nil {
						panicValue = r
						err = newRecoveredPanic(2, r)
					}
				}()
				return task.fn(retryCtx)
			}()

			if fnErr != nil {
				if o.metrics.errorsLabeledVec != nil {
					labels, labelerPanicked := safeErrorLabeler(o.cfg.ErrorLabeler, fnErr)
					if labelerPanicked {
						if o.metrics.panics != nil {
							o.metrics.panics.Inc()
						}
						// Fall back to empty label values on panic
						fallback := make(prometheus.Labels, len(o.cfg.errorLabelNames))
						for _, name := range o.cfg.errorLabelNames {
							fallback[name] = ""
						}
						o.metrics.errorsLabeledVec.With(fallback).Inc()
					} else {
						o.metrics.errorsLabeledVec.With(labels).Inc()
					}
				} else {
					if o.metrics.errors != nil {
						o.metrics.errors.Inc()
					}
					// Check for labeler panics even without labeled vec (labeler set but discovery returned no label names)
					if o.cfg.ErrorLabeler != nil {
						_, labelerPanicked := safeErrorLabeler(o.cfg.ErrorLabeler, fnErr)
						if labelerPanicked && o.metrics.panics != nil {
							o.metrics.panics.Inc()
						}
					}
				}
				if errors.Is(fnErr, context.DeadlineExceeded) && o.metrics.timeouts != nil {
					o.metrics.timeouts.Inc()
				}
				if panicValue != nil && o.metrics.panics != nil {
					o.metrics.panics.Inc()
					o.m.RLock()
					if !o.recoverPanics {
						o.m.RUnlock()
						if o.metrics.failures != nil {
							o.metrics.failures.Inc()
						}
						panic(panicValue) // re-throw
					}
					o.m.RUnlock()
				}
			}
			return fnErr
		})

		// Strip ErrControlBreaker from the joined error: it is a control-flow
		// signal, not a task error, and should not be exposed to the caller.
		if err != nil {
			err = filterControlBreaker(err)
		}
		if err != nil {
			if o.metrics.failures != nil {
				o.metrics.failures.Inc()
			}
		} else {
			if o.metrics.successes != nil {
				o.metrics.successes.Inc()
			}
		}
		return err
	}

	// Add retry count to context
	ctx = context.WithValue(ctx, retryCountKey{}, task.retryCount)

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
				err = newRecoveredPanic(2, r)
			}
		}()
		err = task.fn(ctx)
		return err
	}()

	// Handle errors
	if err != nil {
		if o.metrics.errorsLabeledVec != nil {
			labels, labelerPanicked := safeErrorLabeler(o.cfg.ErrorLabeler, err)
			if labelerPanicked {
				if o.metrics.panics != nil {
					o.metrics.panics.Inc()
				}
				// Fall back to empty label values on panic
				fallback := make(prometheus.Labels, len(o.cfg.errorLabelNames))
				for _, name := range o.cfg.errorLabelNames {
					fallback[name] = ""
				}
				o.metrics.errorsLabeledVec.With(fallback).Inc()
			} else {
				o.metrics.errorsLabeledVec.With(labels).Inc()
			}
		} else {
			if o.metrics.errors != nil {
				o.metrics.errors.Inc()
			}
			// Check for labeler panics even without labeled vec (labeler set but discovery returned no label names)
			if o.cfg.ErrorLabeler != nil {
				_, labelerPanicked := safeErrorLabeler(o.cfg.ErrorLabeler, err)
				if labelerPanicked && o.metrics.panics != nil {
					o.metrics.panics.Inc()
				}
			}
		}
		if errors.Is(err, context.DeadlineExceeded) && o.metrics.timeouts != nil {
			o.metrics.timeouts.Inc()
		}

		// Handle panics
		if panicValue != nil {
			if o.metrics.panics != nil {
				o.metrics.panics.Inc()
			}
			o.m.RLock()
			if !o.recoverPanics {
				o.m.RUnlock()
				if o.metrics.failures != nil {
					o.metrics.failures.Inc()
				}
				panic(panicValue) // re-throw
			}
			o.m.RUnlock()
		}

		if o.metrics.failures != nil {
			o.metrics.failures.Inc()
		}
	} else {
		if o.metrics.successes != nil {
			o.metrics.successes.Inc()
		}
	}

	return err
}
