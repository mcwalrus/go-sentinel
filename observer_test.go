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

func TestObserve_SuccessfulExecution(t *testing.T) {
	observer := NewObserver(testConfig(t))
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	task := &testTask{
		cfg: TaskConfig{
			Timeout:       time.Second,
			MaxRetries:    0,
			Concurrent:    false,
			RecoverPanics: true,
		},
		success: true,
	}

	err := observer.RunTask(task)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Verify metrics
	if got := testutil.ToFloat64(observer.metrics.Successes); got != 1 {
		t.Errorf("Expected Successes=1, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.Errors); got != 0 {
		t.Errorf("Expected Errors=0, got %f", got)
	}
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
		if *family.Name == "test_metrics_observed_duration_seconds" {
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
	observer := NewObserver(testConfig(t))
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	expectedErr := errors.New("task failed")
	task := &testTask{
		cfg: TaskConfig{
			Timeout:       time.Second,
			MaxRetries:    0,
			Concurrent:    false,
			RecoverPanics: false,
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
	if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
		t.Errorf("Expected Successes=0, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.Errors); got != 1 {
		t.Errorf("Expected Errors=1, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.TimeoutErrors); got != 0 {
		t.Errorf("Expected TimeoutErrors=0, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.InFlight); got != 0 {
		t.Errorf("Expected InFlight=0 after completion, got %f", got)
	}
}

func TestObserve_TimeoutHandling(t *testing.T) {
	observer := NewObserver(testConfig(t))
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	task := &testTask{
		cfg: TaskConfig{
			Timeout:       10 * time.Millisecond, // short timeout
			MaxRetries:    0,
			Concurrent:    false,
			RecoverPanics: false,
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
	if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
		t.Errorf("Expected Successes=0, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.Errors); got != 1 {
		t.Errorf("Expected Errors=1, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.TimeoutErrors); got != 1 {
		t.Errorf("Expected TimeoutErrors=1, got %f", got)
	}
}

func TestObserve_PanicRecovery(t *testing.T) {
	t.Run("with panic recovery enabled", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &testTask{
			cfg: TaskConfig{
				Timeout:       time.Second,
				MaxRetries:    0,
				Concurrent:    false,
				RecoverPanics: true, // Enable panic recovery
			},
			fn: func(ctx context.Context) error {
				panic("test panic")
			},
		}

		// This should not panic due to recovery
		err := observer.RunTask(task)

		// Should return nil since panic was recovered
		if err != nil {
			t.Errorf("Expected no error with panic recovery, got %v", err)
		}

		// Verify panic was recorded
		if got := testutil.ToFloat64(observer.metrics.Panics); got != 1 {
			t.Errorf("Expected Panics=1, got %f", got)
		}
	})

	t.Run("with panic recovery disabled", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &testTask{
			cfg: TaskConfig{
				Timeout:       time.Second,
				MaxRetries:    0,
				Concurrent:    false,
				RecoverPanics: false, // Disable panic recovery
			},
			fn: func(ctx context.Context) error {
				panic("test panic")
			},
		}

		// This should panic and be caught by our test
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic to propagate when RecoverPanics=false")
			} else {
				// Verify panic was still recorded even though it propagated
				if got := testutil.ToFloat64(observer.metrics.Panics); got != 1 {
					t.Errorf("Expected Panics=1, got %f", got)
				}
			}
		}()

		observer.RunTask(task)
	})
}

func TestObserve_RetryLogic(t *testing.T) {
	t.Run("successful retry", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		attemptCount := 0
		task := &testTask{
			cfg: TaskConfig{
				Timeout:       time.Second,
				MaxRetries:    2,
				Concurrent:    false,
				RecoverPanics: false,
				RetryStrategy: RetryStrategyImmediate,
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
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 1 {
			t.Errorf("Expected Errors=1, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 1 {
			t.Errorf("Expected Retries=1, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 1 {
			t.Errorf("Expected Successes=1, got %f", got)
		}
	})

	t.Run("exhausted retries", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		attemptCount := 0
		expectedErr := errors.New("persistent failure")
		task := &testTask{
			cfg: TaskConfig{
				Timeout:       10 * time.Millisecond,
				MaxRetries:    2,
				Concurrent:    false,
				RecoverPanics: false,
				RetryStrategy: RetryStrategyImmediate,
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

		if got := testutil.ToFloat64(observer.metrics.Errors); got != 3 {
			t.Errorf("Expected Errors=3, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 3 {
			t.Errorf("Expected Retries=3, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
			t.Errorf("Expected Successes=0, got %f", got)
		}
	})

	t.Run("circuit breaker stops retries", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		attemptCount := 0
		expectedErr := errors.New("circuit breaker error")
		task := &testTask{
			cfg: TaskConfig{
				Timeout:       time.Second,
				MaxRetries:    5,
				Concurrent:    false,
				RetryStrategy: RetryStrategyImmediate,
				RetryCurcuitBreaker: func(err error) bool {
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
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 3 {
			t.Errorf("Expected Retries=3 due to circuit breaker, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 3 {
			t.Errorf("Expected Errors=3 due to circuit breaker, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
			t.Errorf("Expected Successes=0 due to circuit breaker, got %f", got)
		}
	})
}

func TestObserve_RetryStrategy(t *testing.T) {
	observer := NewObserver(testConfig(t))
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	t.Run("retry strategy is called with correct parameters", func(t *testing.T) {
		retryStrategyCalls := []int{}
		task := &testTask{
			cfg: TaskConfig{
				Timeout:    time.Second,
				MaxRetries: 2,
				Concurrent: false,
				RetryStrategy: func(retryAttempt int) time.Duration {
					retryStrategyCalls = append(retryStrategyCalls, retryAttempt)
					return 1 * time.Millisecond
				},
			},
			fn: func(ctx context.Context) error {
				return errors.New("always fails")
			},
		}

		observer.RunTask(task)

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
	observer := NewObserver(testConfig(t))
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	var maxInFlight float64
	var wg sync.WaitGroup

	// Run multiple concurrent tasks to test in-flight metrics
	numTasks := 5
	wg.Add(numTasks)

	for i := 0; i < numTasks; i++ {
		go func() {
			defer wg.Done()
			task := &testTask{
				cfg: TaskConfig{
					Timeout:    time.Second,
					Concurrent: false,
				},
				fn: func(ctx context.Context) error {
					// Check in-flight count during execution
					current := testutil.ToFloat64(observer.metrics.InFlight)
					if current > maxInFlight {
						maxInFlight = current
					}
					time.Sleep(50 * time.Millisecond) // Simulate work
					return nil
				},
			}
			observer.RunTask(task)
		}()
	}

	wg.Wait()

	// After all tasks complete, in-flight should be 0
	if got := testutil.ToFloat64(observer.metrics.InFlight); got != 0 {
		t.Errorf("Expected InFlight=0 after all tasks complete, got %f", got)
	}

	// We should have seen some tasks in flight concurrently
	if maxInFlight == 0 {
		t.Error("Expected to see some tasks in flight during execution")
	}
}

func TestObserve_ConcurrentExecution(t *testing.T) {
	observer := NewObserver(testConfig(t))
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	t.Run("concurrent task execution", func(t *testing.T) {
		var wg sync.WaitGroup
		numGoroutines := 10
		wg.Add(numGoroutines)

		executed := int32(0)
		task := &testTask{
			cfg: TaskConfig{
				Timeout:    time.Second,
				Concurrent: true,
			},
			fn: func(ctx context.Context) error {
				defer wg.Done()
				atomic.AddInt32(&executed, 1)
				return nil
			},
		}

		for i := 0; i < numGoroutines; i++ {
			go func() {
				_ = observer.RunTask(task)
			}()
		}

		wg.Wait()

		if got := atomic.LoadInt32(&executed); got != int32(numGoroutines) {
			t.Errorf("Expected %d executions, got %d", numGoroutines, got)
		}

		// Verify all tasks were recorded as successes
		if got := testutil.ToFloat64(observer.metrics.Successes); got != float64(numGoroutines) {
			t.Errorf("Expected Successes=%d, got %f", numGoroutines, got)
		}
	})
}

func TestObserve_MetricsRecording(t *testing.T) {
	observer := NewObserver(testConfig(t))
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
				cfg: defaultTaskConfig(),
				fn: func(ctx context.Context) error {
					return nil
				},
			},
			wantErr: false,
			validate: func(t *testing.T, observer *Observer) {
				if got := testutil.ToFloat64(observer.metrics.Successes); got != 1 {
					t.Errorf("Expected Successes=1, got %f", got)
				}
			},
		},
		{
			name: "failed task",
			task: &testTask{
				cfg: defaultTaskConfig(),
				fn: func(ctx context.Context) error {
					return errors.New("task failed")
				},
			},
			wantErr: true,
			validate: func(t *testing.T, observer *Observer) {
				if got := testutil.ToFloat64(observer.metrics.Errors); got != 1 {
					t.Errorf("Expected Errors=1, got %f", got)
				}
			},
		},
		{
			name: "task with panic (recovered)",
			task: &testTask{
				cfg: TaskConfig{
					RecoverPanics: true,
				},
				fn: func(ctx context.Context) error {
					panic("test panic")
				},
			},
			wantErr: false,
			validate: func(t *testing.T, observer *Observer) {
				if got := testutil.ToFloat64(observer.metrics.Panics); got != 1 {
					t.Errorf("Expected Panics=1, got %f", got)
				}
			},
		},
	}

	for i, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			observer := NewObserver(testConfig(t))
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
				if *family.Name == "test_metrics_observed_duration_seconds" {
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
	observer := NewObserver(testConfig(t))
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
	if got := testutil.ToFloat64(observer.metrics.TimeoutErrors); got != 1 {
		t.Errorf("Expected TimeoutErrors=1, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.Errors); got != 1 {
		t.Errorf("Expected Errors=1, got %f", got)
	}
	if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
		t.Errorf("Expected Successes=0, got %f", got)
	}
}
