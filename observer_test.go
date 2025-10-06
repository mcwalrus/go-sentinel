package sentinel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// metricsCounts is a helper struct for verifying metrics counts.
type metricsCounts struct {
	Successes     float64
	Errors        float64
	TimeoutErrors float64
	Panics        float64
	Retries       float64
}

// Verify verifies an observer against it's expected metrics counts.
func Verify(t *testing.T, observer *Observer, m metricsCounts) {
	t.Helper()
	if got := testutil.ToFloat64(observer.metrics.Successes); got != m.Successes {
		t.Errorf("Expected Successes=%f, got %f", m.Successes, got)
	}
	if got := testutil.ToFloat64(observer.metrics.Errors); got != m.Errors {
		t.Errorf("Expected Errors=%f, got %f", m.Errors, got)
	}
	if got := testutil.ToFloat64(observer.metrics.TimeoutErrors); got != m.TimeoutErrors {
		t.Errorf("Expected TimeoutErrors=%f, got %f", m.TimeoutErrors, got)
	}
	if got := testutil.ToFloat64(observer.metrics.Panics); got != m.Panics {
		t.Errorf("Expected Panics=%f, got %f", m.Panics, got)
	}
	if got := testutil.ToFloat64(observer.metrics.Retries); got != m.Retries {
		t.Errorf("Expected Retries=%f, got %f", m.Retries, got)
	}
}

// testTask is a test implementation of the Task interface.
// It allows for task configuration, retry count, success/failure, and error handling.
// It can execute either a custom function (fn) or use the built-in test logic.
// See testTask.Execute for how the task is executed.
type testTask struct {
	cfg      TaskConfig
	nRetries int
	success  bool
	err      error
	tryPanic bool
	fn       func(ctx context.Context) error
}

var _ Task = (*testTask)(nil)

func (t *testTask) Config() TaskConfig {
	return t.cfg
}

func (t *testTask) Execute(ctx context.Context) error {
	if t.fn != nil {
		return t.fn(ctx)
	}
	if t.tryPanic {
		panic("test panic")
	}
	if t.nRetries > 0 {
		t.nRetries--
		return t.err
	}
	if t.success {
		return nil
	}
	return t.err
}

func TestObserver_DefaultConfig(t *testing.T) {
	observer := NewObserver()
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	expected := []string{
		"sentinel_in_flight",
		"sentinel_successes_total",
		"sentinel_errors_total",
		"sentinel_timeouts_total",
		"sentinel_panics_total",
		"sentinel_durations_seconds",
		"sentinel_retries_total",
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	foundMetrics := make(map[string]bool)
	for _, family := range families {
		foundMetrics[*family.Name] = true
	}

	for _, expectedName := range expected {
		if !foundMetrics[expectedName] {
			t.Errorf("Expected metric %s not found", expectedName)
		}
	}
}

func TestObserver_ZeroConfig(t *testing.T) {
	observer := NewObserver()
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	expected := []string{
		"sentinel_in_flight",
		"sentinel_successes_total",
		"sentinel_errors_total",
		"sentinel_timeouts_total",
		"sentinel_panics_total",
		"sentinel_durations_seconds",
		"sentinel_retries_total",
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	foundMetrics := make(map[string]bool)
	for _, family := range families {
		foundMetrics[*family.Name] = true
	}

	for _, expectedName := range expected {
		if !foundMetrics[expectedName] {
			t.Errorf("Expected metric %s not found", expectedName)
		}
	}
}

func TestObserve_Register(t *testing.T) {
	observer := NewObserver()
	registry := prometheus.NewRegistry()

	// First registration should succeed
	err := observer.Register(registry)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Re-registration should return an error
	if err := observer.Register(registry); err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestObserve_SuccessfulExecution(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	task := &testTask{
		cfg: TaskConfig{
			Timeout:    time.Second,
			MaxRetries: 0,
		},
		success: true,
	}

	err := observer.RunTask(task)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify metrics
	Verify(t, observer, metricsCounts{
		Successes:     1,
		Errors:        0,
		TimeoutErrors: 0,
		Panics:        0,
		Retries:       0,
	})

	if got := testutil.ToFloat64(observer.metrics.InFlight); got != 0 {
		t.Errorf("Expected InFlight=0 after completion, got %f", got)
	}

	// Verify duration was recorded
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	var histogramSampleCount uint64
	for _, family := range families {
		if *family.Name == "test_metrics_durations_seconds" {
			if len(family.Metric) > 0 && family.Metric[0].Histogram != nil {
				histogramSampleCount = *family.Metric[0].Histogram.SampleCount
				break
			}
		}
	}

	if histogramSampleCount != 1 {
		t.Errorf("Expected ObservedRuntimes count=1, got %d", histogramSampleCount)
	}
}

func TestObserve_ErrorHandling(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	expectedErr := errors.New("task failed")
	task := &testTask{
		cfg: TaskConfig{
			Timeout:    time.Second,
			MaxRetries: 0,
		},
		fn: func(ctx context.Context) error {
			return expectedErr
		},
	}

	err := observer.RunTask(task)

	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	// Verify metrics
	Verify(t, observer, metricsCounts{
		Successes:     0,
		Errors:        1,
		TimeoutErrors: 0,
		Panics:        0,
		Retries:       0,
	})
}

func TestObserve_RunFunc(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	err := observer.RunFunc(func() error {
		return errors.New("task failed")
	})
	if err == nil {
		t.Errorf("Expected error, got nil")
	}

	Verify(t, observer, metricsCounts{
		Successes:     0,
		Errors:        1,
		TimeoutErrors: 0,
		Panics:        0,
		Retries:       0,
	})
}

func TestObserve_TimeoutHandling(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	task := &testTask{
		cfg: TaskConfig{
			Timeout:    10 * time.Millisecond, // short timeout
			MaxRetries: 0,
		},
		fn: func(ctx context.Context) error {
			time.Sleep(20 * time.Millisecond)
			return ctx.Err() // expect: context.DeadlineExceeded
		},
	}

	err := observer.RunTask(task)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	// Verify metrics
	Verify(t, observer, metricsCounts{
		Successes:     0,
		Errors:        1,
		TimeoutErrors: 1,
		Panics:        0,
		Retries:       0,
	})
}

func TestObserve_PanicRecovery(t *testing.T) {
	t.Run("with panic recovery enabled (default)", func(t *testing.T) {
		observer := testObserver(t)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &testTask{
			cfg: TaskConfig{
				Timeout:    time.Second,
				MaxRetries: 0,
			},
			fn: func(ctx context.Context) error {
				panic("test panic")
			},
		}

		// This should not panic due to recovery (default behavior)
		err := observer.RunTask(task)

		// Should return an error indicating panic occurred
		if err == nil {
			t.Errorf("Expected error indicating panic occurred, got nil")
		}
		if err != nil && err.Error() != "panic occurred for task execution" {
			t.Errorf("Expected panic error message, got %v", err)
		}

		// Verify panic was recorded
		Verify(t, observer, metricsCounts{
			Successes:     0,
			Errors:        1,
			TimeoutErrors: 0,
			Panics:        1,
			Retries:       0,
		})
	})

	t.Run("with panic recovery disabled via ObserverOption", func(t *testing.T) {
		observer := NewObserver(WithPanicRecovery(false))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &testTask{
			cfg: TaskConfig{
				Timeout:    time.Second,
				MaxRetries: 0,
			},
			fn: func(ctx context.Context) error {
				panic("test panic")
			},
		}

		// This should panic and be caught by our test
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic to propagate when WithPanicRecovery(false)")
			} else {
				// Verify panic was still recorded even though it propagated
				Verify(t, observer, metricsCounts{
					Successes:     0,
					Errors:        1,
					TimeoutErrors: 0,
					Panics:        1,
					Retries:       0,
				})
			}
		}()

		_ = observer.RunTask(task)
	})
}

func TestObserve_RetryLogic(t *testing.T) {
	t.Run("successful retry", func(t *testing.T) {
		observer := testObserver(t)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		attemptCount := 0
		task := &testTask{
			cfg: TaskConfig{
				Timeout:    time.Second,
				MaxRetries: 2,
			},
			fn: func(ctx context.Context) error {
				attemptCount++
				if attemptCount == 1 {
					return errors.New("first attempt fails")
				}
				return nil
			},
		}

		err := observer.RunTask(task)

		if err != nil {
			t.Errorf("Expected no error after successful retry, got %v", err)
		}
		if attemptCount != 2 {
			t.Errorf("Expected 2 attempts, got %d", attemptCount)
		}

		// Verify metrics: one retry attempt was made
		Verify(t, observer, metricsCounts{
			Successes:     1,
			Errors:        1,
			TimeoutErrors: 0,
			Panics:        0,
			Retries:       1,
		})
	})

	t.Run("exhausted retries", func(t *testing.T) {
		observer := testObserver(t)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		attemptCount := 0
		expectedErr := errors.New("persistent failure")
		task := &testTask{
			cfg: TaskConfig{
				Timeout:    10 * time.Millisecond,
				MaxRetries: 2,
			},
			fn: func(ctx context.Context) error {
				attemptCount++
				return expectedErr
			},
		}

		// Run the task in a goroutine incase of infinite loop
		done := make(chan error)
		go func() {
			done <- observer.RunTask(task)
		}()
		var err error
		select {
		case err = <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("Expected error to be returned, got timeout")
		}

		if attemptCount != 3 {
			t.Errorf("Expected 3 attempts, got %d", attemptCount)
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("Expected error to contain %v, got %v", expectedErr, err)
		}

		Verify(t, observer, metricsCounts{
			Successes:     0,
			Errors:        3,
			TimeoutErrors: 0,
			Panics:        0,
			Retries:       2,
		})
	})

	t.Run("circuit breaker stops retries", func(t *testing.T) {
		observer := testObserver(t)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		attemptCount := 0
		expectedErr := errors.New("circuit breaker error")
		task := &testTask{
			cfg: TaskConfig{
				Timeout:    time.Second,
				MaxRetries: 5,
				RetryCircuitBreaker: func(err error) bool {
					// Break circuit on specific error
					return errors.Is(err, expectedErr)
				},
			},
			fn: func(ctx context.Context) error {
				attemptCount++
				if attemptCount == 3 {
					return expectedErr
				}
				return errors.New("other error")
			},
		}

		err := observer.RunTask(task)

		if !errors.Is(err, expectedErr) {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
		if attemptCount != 3 {
			t.Errorf("Expected 3 attempts due to circuit breaker, got %d", attemptCount)
		}

		// Verify three retries were attempted due to circuit breaker
		Verify(t, observer, metricsCounts{
			Successes:     0,
			Errors:        3,
			TimeoutErrors: 0,
			Panics:        0,
			Retries:       3,
		})
	})
}

func TestObserve_RetryStrategy(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	t.Run("retry strategy is called with correct parameters", func(t *testing.T) {
		retryStrategyCalls := []int{}
		task := &testTask{
			cfg: TaskConfig{
				Timeout:    time.Second,
				MaxRetries: 2,
				RetryStrategy: func(retryAttempt int) time.Duration {
					retryStrategyCalls = append(retryStrategyCalls, retryAttempt)
					return 1 * time.Millisecond
				},
			},
			fn: func(ctx context.Context) error {
				return errors.New("always fails")
			},
		}

		err := observer.RunTask(task)
		if err == nil {
			t.Errorf("Expected error, got nil")
		}

		if len(retryStrategyCalls) != 2 {
			t.Fatalf("Expected 1 retry strategy call, got %d", len(retryStrategyCalls))
		}
		if retryStrategyCalls[0] != 0 && retryStrategyCalls[1] != 1 {
			t.Errorf(
				"Expected retry strategy called with args: 0 and 1",
			)
		}
	})
}

func TestObserve_InFlightMetrics(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	t.Run("concurrent in-flight tracking", func(t *testing.T) {
		const numTasks = 10
		const activeDuration = 30 * time.Millisecond
		startBarrier := make(chan struct{})
		var wg sync.WaitGroup

		wg.Add(numTasks)

		// Start all tasks concurrently
		for range numTasks {
			go func() {
				defer wg.Done()

				task := &testTask{
					cfg: TaskConfig{
						Timeout: time.Second,
					},
					fn: func(ctx context.Context) error {
						<-startBarrier
						time.Sleep(activeDuration)

						return nil
					},
				}

				err := observer.RunTask(task)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}()
		}
		// Wait for all tasks to start
		time.Sleep(10 * time.Millisecond)

		// Verify that we have 10 tasks in flight
		inFlightCount := testutil.ToFloat64(observer.metrics.InFlight)
		if inFlightCount != numTasks {
			t.Errorf("Expected InFlight=%d when all tasks active, got %f", numTasks, inFlightCount)
		}

		// Release all tasks and wait for them to complete
		close(startBarrier)
		wg.Wait()

		// Verify that in-flight count returns to 0
		finalInFlight := testutil.ToFloat64(observer.metrics.InFlight)
		if finalInFlight != 0 {
			t.Errorf("Expected InFlight=0 after all tasks complete, got %f", finalInFlight)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != numTasks {
			t.Errorf("Expected Successes=%d, got %f", numTasks, got)
		}
	})
}

func TestObserve_Concurrent(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	t.Run("concurrent task execution", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 10
		wg.Add(numGoroutines)

		executed := int32(0)
		task := &testTask{
			cfg: TaskConfig{
				Timeout: time.Second,
			},
			fn: func(ctx context.Context) error {
				defer wg.Done()
				atomic.AddInt32(&executed, 1)
				return nil
			},
		}

		for range numGoroutines {
			go func() {
				_ = observer.RunTask(task)
			}()
		}

		wg.Wait()

		if got := atomic.LoadInt32(&executed); got != int32(numGoroutines) {
			t.Errorf("Expected %d executions, got %d", numGoroutines, got)
		}

		// Verify all tasks were recorded as successes
		Verify(t, observer, metricsCounts{
			Successes:     float64(numGoroutines),
			Errors:        0,
			TimeoutErrors: 0,
			Panics:        0,
			Retries:       0,
		})
	})
}

func TestObserve_MetricsRecording(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	// Test that metrics are properly recorded for various scenarios
	scenarios := []struct {
		name     string
		task     *testTask
		wantErr  bool
		validate func(t *testing.T, observer *Observer)
	}{
		{
			name: "successful task",
			task: &testTask{
				fn: func(ctx context.Context) error {
					return nil
				},
			},
			wantErr: false,
			validate: func(t *testing.T, observer *Observer) {
				Verify(t, observer, metricsCounts{
					Successes:     1,
					Errors:        0,
					TimeoutErrors: 0,
					Panics:        0,
					Retries:       0,
				})
			},
		},
		{
			name: "failed task",
			task: &testTask{
				fn: func(ctx context.Context) error {
					return errors.New("task failed")
				},
			},
			wantErr: true,
			validate: func(t *testing.T, observer *Observer) {
				Verify(t, observer, metricsCounts{
					Successes:     0,
					Errors:        1,
					TimeoutErrors: 0,
					Panics:        0,
					Retries:       0,
				})
			},
		},
		{
			name: "task with panic (recovered)",
			task: &testTask{
				cfg: TaskConfig{},
				fn: func(ctx context.Context) error {
					panic("test panic")
				},
			},
			wantErr: true,
			validate: func(t *testing.T, observer *Observer) {
				Verify(t, observer, metricsCounts{
					Successes:     0,
					Errors:        1,
					TimeoutErrors: 0,
					Panics:        1,
					Retries:       0,
				})
			},
		},
		{
			name: "Multiple retries task with error returned",
			task: &testTask{
				cfg: TaskConfig{
					MaxRetries: 3,
				},
				fn: func(ctx context.Context) error {
					return errors.New("test error")
				},
			},
			wantErr: true,
			validate: func(t *testing.T, observer *Observer) {
				time.Sleep(10 * time.Millisecond)
				Verify(t, observer, metricsCounts{
					Successes:     0,
					Errors:        4,
					TimeoutErrors: 0,
					Panics:        0,
					Retries:       3,
				})
			},
		},
	}

	for i, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Logf("Running scenario: %s", scenario.name)

			observer := testObserver(t)
			registry := prometheus.NewRegistry()
			observer.MustRegister(registry)

			err := observer.RunTask(scenario.task)

			if scenario.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !scenario.wantErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			scenario.validate(t, observer)

			// Verify duration was recorded for all scenarios
			families, err := registry.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			var histogramSampleCount uint64
			for _, family := range families {
				if *family.Name == "test_metrics_durations_seconds" {
					if len(family.Metric) > 0 && family.Metric[0].Histogram != nil {
						histogramSampleCount = *family.Metric[0].Histogram.SampleCount
						break
					}
				}
			}

			if histogramSampleCount < 1 {
				t.Errorf("Test %d: Expected at least 1 duration sample, got %d", i, histogramSampleCount)
			}
		})
	}
}

func TestObserve_ContextTimeout(t *testing.T) {
	observer := testObserver(t)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	task := &testTask{
		cfg: TaskConfig{
			Timeout: 20 * time.Millisecond,
		},
		fn: func(ctx context.Context) error {
			select {
			case <-ctx.Done(): // Respect context cancellation
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		},
	}

	start := time.Now()
	err := observer.RunTask(task)
	duration := time.Since(start)

	// Should timeout quickly
	if duration > 50*time.Millisecond {
		t.Errorf("Expected task to timeout quickly, took %v", duration)
	}

	// Should return a deadline exceeded error
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	// Verify timeout was recorded
	Verify(t, observer, metricsCounts{
		Successes:     0,
		Errors:        1,
		TimeoutErrors: 1,
		Panics:        0,
		Retries:       0,
	})
}

func TestMultipleObservers(t *testing.T) {
	observer := NewObserver(
		WithNamespace("test"),
		WithSubsystem("observer1"),
		WithDescription("test operations"),
		WithBucketDurations([]float64{0.01, 0.1, 1, 10, 100}),
	)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	observer2 := NewObserver(
		WithNamespace("test"),
		WithSubsystem("observer2"),
		WithDescription("test operations"),
		WithBucketDurations([]float64{0.01, 0.1, 1, 10, 100}),
	)
	registry2 := prometheus.NewRegistry()
	observer2.MustRegister(registry2)

	_ = observer.RunFunc(func() error {
		return nil
	})

	_ = observer2.RunFunc(func() error {
		return nil
	})

	Verify(t, observer, metricsCounts{
		Successes:     1,
		Errors:        0,
		TimeoutErrors: 0,
		Panics:        0,
		Retries:       0,
	})
	Verify(t, observer2, metricsCounts{
		Successes:     1,
		Errors:        0,
		TimeoutErrors: 0,
		Panics:        0,
		Retries:       0,
	})
}

func Benchmark_ObserverRun(b *testing.B) {

	observer := NewObserver(
		WithNamespace("test"),
		WithSubsystem("metrics"),
		WithDescription("test operations"),
		WithBucketDurations([]float64{0.01, 0.1, 1, 10, 100}),
	)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	b.Run("simple function", func(b *testing.B) {
		cfg := TaskConfig{}
		for i := 0; i < b.N; i++ {
			_ = observer.Run(cfg, func(ctx context.Context) error {
				return nil
			})
		}
	})

	b.Run("function with work", func(b *testing.B) {
		cfg := TaskConfig{}
		for i := 0; i < b.N; i++ {
			_ = observer.Run(cfg, func(ctx context.Context) error {
				time.Sleep(time.Microsecond)
				return nil
			})
		}
	})
}

func TestObserver_TestPanicHandling(t *testing.T) {
	t.Run("with panic recovery enabled (default)", func(t *testing.T) {
		observer := testObserver(t)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)
		task := &testTask{
			cfg: TaskConfig{
				MaxRetries: 0,
			},
			fn: func(ctx context.Context) error {
				panic("test panic")
			},
		}

		err := observer.RunTask(task)
		if err == nil {
			t.Errorf("Expected error indicating panic occurred, got nil")
		}
		if err != nil && err.Error() != "panic occurred for task execution" {
			t.Errorf("Expected panic error message, got %v", err)
		}

		Verify(t, observer, metricsCounts{
			Successes:     0,
			Errors:        1,
			TimeoutErrors: 0,
			Panics:        1,
			Retries:       0,
		})
	})

	t.Run("with panic recovery disabled via ObserverOption", func(t *testing.T) {
		observer := NewObserver(WithPanicRecovery(false))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &testTask{
			cfg: TaskConfig{
				MaxRetries: 0, // No retries to avoid multiple panic attempts
			},
			fn: func(ctx context.Context) error {
				panic("test panic")
			},
		}

		// This should panic and be caught by our test
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic to propagate when WithPanicRecovery(false)")
			} else {
				// Verify panic was still recorded even though it propagated
				Verify(t, observer, metricsCounts{
					Successes:     0,
					Errors:        1,
					TimeoutErrors: 0,
					Panics:        1,
					Retries:       0,
				})
			}
		}()

		_ = observer.RunTask(task)
	})
}
