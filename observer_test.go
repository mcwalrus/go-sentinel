package sentinel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"

	"github.com/mcwalrus/go-sentinel/circuit"
	"github.com/mcwalrus/go-sentinel/retry"
)

type metricsCounts struct {
	Successes float64
	Failures  float64
	Timeouts  float64
	Errors    float64
	Panics    float64
	Retries   float64
}

// Verify validates observer metrics counts
func Verify(t *testing.T, observer *Observer, m metricsCounts) {
	t.Helper()
	if got := testutil.ToFloat64(observer.metrics.successes); got != m.Successes {
		t.Errorf("Expected Successes=%f, got %f", m.Successes, got)
	}
	if got := testutil.ToFloat64(observer.metrics.failures); got != m.Failures {
		t.Errorf("Expected Failures=%f, got %f", m.Failures, got)
	}
	if got := testutil.ToFloat64(observer.metrics.errors); got != m.Errors {
		t.Errorf("Expected Errors=%f, got %f", m.Errors, got)
	}
	if got := testutil.ToFloat64(observer.metrics.panics); got != m.Panics {
		t.Errorf("Expected Panics=%f, got %f", m.Panics, got)
	}
	if got := testutil.ToFloat64(observer.metrics.timeouts); got != m.Timeouts {
		t.Errorf("Expected Timeouts=%f, got %f", m.Timeouts, got)
	}
	if got := testutil.ToFloat64(observer.metrics.retries); got != m.Retries {
		t.Errorf("Expected Retries=%f, got %f", m.Retries, got)
	}
}

type testTask struct {
	nRetries int
	success  bool
	err      error
	tryPanic bool
	fn       func(_ context.Context) error
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
	t.Parallel()

	observer := NewObserver(nil)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	expected := []string{
		"sentinel_in_flight",
		"sentinel_success_total",
		"sentinel_failures_total",
		"sentinel_errors_total",
		"sentinel_panics_total",
		"sentinel_timeouts_total",
		"sentinel_retries_total",
		"sentinel_pending_total",
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}
	if len(families) != len(expected) {
		t.Errorf(
			"Expected %d metrics, got %d",
			len(expected), len(families),
		)
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

	t.Log("Verify histogram buckets")
	if _, ok := foundMetrics["sentinel_durations_seconds"]; ok {
		t.Errorf("Expected not to find 'durations_seconds' on default observer config")
	}
}

func TestObserver_CustomConfig(t *testing.T) {
	t.Parallel()

	observer := NewObserver(
		[]float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
		WithNamespace("myapp"),
		WithSubsystem("service"),
	)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	expected := []string{
		"myapp_service_in_flight",
		"myapp_service_success_total",
		"myapp_service_failures_total",
		"myapp_service_errors_total",
		"myapp_service_panics_total",
		"myapp_service_timeouts_total",
		"myapp_service_retries_total",
		"myapp_service_durations_seconds",
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

	t.Log("Verify histogram buckets")
	if _, ok := foundMetrics["myapp_service_durations_seconds"]; !ok {
		t.Errorf("Expected to find 'durations_seconds' on observer config with histogram buckets")
	}
}

func TestObserver_UnconfiguredObserver(t *testing.T) {
	t.Parallel()

	var expectPanic = func(t *testing.T, msg string) {
		if r := recover(); r == nil {
			t.Errorf("Expected panic: %s", msg)
		}
	}

	t.Run("observer nil pointer", func(t *testing.T) {
		t.Parallel()
		var observer *Observer

		t.Run("Run", func(t *testing.T) {
			t.Parallel()
			defer expectPanic(t, "observer.Run")
			_ = observer.Run(func() error { return nil })
		})

		t.Run("RunFunc", func(t *testing.T) {
			t.Parallel()
			defer expectPanic(t, "observer.RunFunc")
			_ = observer.RunFunc(func(_ context.Context) error { return nil })
		})
	})

	t.Run("observer variable declaration", func(t *testing.T) {
		t.Parallel()
		observer := &Observer{}

		t.Run("Run", func(t *testing.T) {
			t.Parallel()
			defer expectPanic(t, "observer.Run")
			_ = observer.Run(func() error { return nil })
		})

		t.Run("RunFunc", func(t *testing.T) {
			t.Parallel()
			defer expectPanic(t, "observer.RunFunc")
			_ = observer.RunFunc(func(_ context.Context) error { return nil })
		})
	})
}

func TestObserve_Register(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)
	registry := prometheus.NewRegistry()

	t.Log("First registration should succeed")
	err := observer.Register(registry)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	t.Log("Re-registration should return an error")
	if err := observer.Register(registry); err == nil {
		t.Errorf("Expected error, got nil")
	}
}

func TestObserve_MustRegister(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)
	registry := prometheus.NewRegistry()

	t.Log("First registration should succeed")
	observer.MustRegister(registry)

	t.Log("Re-registration should panic")
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic, got nil")
		}
	}()
	observer.MustRegister(registry)
}

func TestObserve_SuccessfulExecution(t *testing.T) {
	t.Parallel()

	observer := NewObserver([]float64{1, 3, 5})
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	observer.UseConfig(ObserverConfig{
		Timeout:    time.Second,
		MaxRetries: 0,
	})

	task := &testTask{
		success: true,
	}

	err := observer.RunFunc(task.Execute)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	Verify(t, observer, metricsCounts{
		Successes: 1,
		Failures:  0,
		Errors:    0,
		Timeouts:  0,
		Panics:    0,
		Retries:   0,
	})

	if got := testutil.ToFloat64(observer.metrics.inFlight); got != 0 {
		t.Errorf("Expected InFlight=0 after completion, got %f", got)
	}

	t.Log("Verify duration was recorded")
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	var histogramSampleCount uint64
	for _, family := range families {
		if *family.Name == "sentinel_durations_seconds" {
			if len(family.Metric) > 0 && family.Metric[0].Histogram != nil {
				histogramSampleCount = *family.Metric[0].Histogram.SampleCount
				break
			}
		}
	}

	if histogramSampleCount != 1 {
		t.Errorf("Expected Durations count=1, got %d", histogramSampleCount)
	}
}

func TestObserve_ErrorHandling(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)
	observer.UseConfig(ObserverConfig{
		Timeout: time.Second,
	})

	expectedErr := errors.New("task failed")
	task := &testTask{
		fn: func(_ context.Context) error {
			return expectedErr
		},
	}

	err := observer.RunFunc(task.Execute)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	Verify(t, observer, metricsCounts{
		Successes: 0,
		Failures:  1,
		Errors:    1,
		Timeouts:  0,
		Panics:    0,
		Retries:   0,
	})
}

func TestObserve_RunFunc(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)

	err := observer.Run(func() error {
		return errors.New("task failed")
	})
	if err == nil {
		t.Errorf("Expected error, got nil")
	}

	Verify(t, observer, metricsCounts{
		Successes: 0,
		Failures:  1,
		Errors:    1,
		Timeouts:  0,
		Panics:    0,
		Retries:   0,
	})
}

func TestObserve_TimeoutHandling(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)

	observer.UseConfig(ObserverConfig{
		Timeout:    10 * time.Millisecond,
		MaxRetries: 0,
	})

	task := &testTask{
		fn: func(ctx context.Context) error {
			time.Sleep(20 * time.Millisecond)
			return ctx.Err()
		},
	}

	err := observer.RunFunc(task.Execute)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	// Verify metrics
	Verify(t, observer, metricsCounts{
		Successes: 0,
		Failures:  1,
		Errors:    1,
		Timeouts:  1,
		Panics:    0,
		Retries:   0,
	})
}

func TestObserve_PanicRecovery(t *testing.T) {
	t.Parallel()

	t.Run("with panic recovery enabled (default)", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			Timeout:    time.Second,
			MaxRetries: 0,
		})

		task := &testTask{
			fn: func(_ context.Context) error {
				panic("test panic")
			},
		}

		err := observer.RunFunc(task.Execute)
		if _, ok := IsPanicError(err); !ok {
			t.Errorf("Expected panic error, got %v", err)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    1,
			Retries:   0,
		})
	})

	t.Run("with panic recovery disabled via ObserverOption", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			Timeout: time.Second,
		})

		task := &testTask{
			fn: func(_ context.Context) error {
				panic("test panic")
			},
		}

		observer.DisablePanicRecovery(true)

		// This should panic and be caught by our test
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic to propagate when PanicRecovery(recover bool)")
			} else {
				// Verify panic was still recorded even though it propagated
				Verify(t, observer, metricsCounts{
					Successes: 0,
					Failures:  1,
					Errors:    1,
					Timeouts:  0,
					Panics:    1,
					Retries:   0,
				})
			}
		}()

		_ = observer.RunFunc(task.Execute)
	})
}

func TestObserve_RetryLogic(t *testing.T) {
	t.Parallel()

	t.Run("successful retry", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:    10 * time.Millisecond,
			MaxRetries: 2,
		})

		attemptCount := 0
		task := &testTask{
			fn: func(_ context.Context) error {
				attemptCount++
				if attemptCount == 1 {
					return errors.New("first attempt fails")
				}
				return nil
			},
		}

		err := observer.RunFunc(task.Execute)

		if err != nil {
			t.Errorf("Expected no error after successful retry, got %v", err)
		}
		if attemptCount != 2 {
			t.Errorf("Expected 2 attempts, got %d", attemptCount)
		}

		// Verify metrics: one retry attempt was made
		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   1,
		})
	})

	t.Run("exhausted retries", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:    10 * time.Millisecond,
			MaxRetries: 2,
		})

		attemptCount := 0
		expectedErr := errors.New("persistent failure")
		task := &testTask{
			fn: func(_ context.Context) error {
				attemptCount++
				return expectedErr
			},
		}

		// Run the task in a goroutine incase of infinite loop
		done := make(chan error)
		go func() {
			done <- observer.RunFunc(task.Execute)
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
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Timeouts:  0,
			Panics:    0,
			Retries:   2,
		})
	})

	t.Run("custom circuit breaker stops retries", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		expectedErr := errors.New("circuit breaker error")
		observer.UseConfig(ObserverConfig{
			Timeout:    time.Second,
			MaxRetries: 5,
			RetryBreaker: func(err error) bool {
				return errors.Is(err, expectedErr)
			},
		})

		attemptCount := 0
		task := &testTask{
			fn: func(_ context.Context) error {
				attemptCount++
				if attemptCount == 3 {
					return expectedErr
				}
				return errors.New("other error")
			},
		}

		err := observer.RunFunc(task.Execute)

		if !errors.Is(err, expectedErr) {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
		if attemptCount != 3 {
			t.Errorf("Expected 3 attempts due to circuit breaker, got %d", attemptCount)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Timeouts:  0,
			Panics:    0,
			Retries:   2,
		})
	})
}

func TestRetryCount(t *testing.T) {
	t.Parallel()

	t.Run("retry count in context", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			Timeout:    10 * time.Millisecond,
			MaxRetries: 3,
		})

		var retryCounts []int
		task := &testTask{
			fn: func(ctx context.Context) error {
				retryCount := RetryCount(ctx)
				retryCounts = append(retryCounts, retryCount)
				if retryCount < 2 {
					return errors.New("retry needed")
				}
				return nil
			},
		}

		err := observer.RunFunc(task.Execute)
		if err != nil {
			t.Errorf("Expected no error after successful retry, got %v", err)
		}

		// Should have 3 attempts: initial (0), first retry (1), second retry (2)
		if len(retryCounts) != 3 {
			t.Errorf("Expected 3 attempts, got %d", len(retryCounts))
		}
		if retryCounts[0] != 0 {
			t.Errorf("Expected first attempt retry count to be 0, got %d", retryCounts[0])
		}
		if retryCounts[1] != 1 {
			t.Errorf("Expected second attempt retry count to be 1, got %d", retryCounts[1])
		}
		if retryCounts[2] != 2 {
			t.Errorf("Expected third attempt retry count to be 2, got %d", retryCounts[2])
		}
	})

	t.Run("retry count without retries", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			Timeout:    10 * time.Millisecond,
			MaxRetries: 0,
		})

		var retryCount int
		task := &testTask{
			fn: func(ctx context.Context) error {
				retryCount = RetryCount(ctx)
				return nil
			},
		}

		err := observer.RunFunc(task.Execute)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if retryCount != 0 {
			t.Errorf("Expected retry count to be 0 for first attempt, got %d", retryCount)
		}
	})

	t.Run("retry count with context without value", func(t *testing.T) {
		t.Parallel()

		// Test RetryCount with a context that doesn't have the retry count value
		ctx := context.Background()
		count := RetryCount(ctx)
		if count != 0 {
			t.Errorf("Expected RetryCount to return 0 for context without value, got %d", count)
		}
	})
}

func TestObserve_RetryStrategy(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)

	t.Run("retry strategy is called with correct parameters", func(t *testing.T) {
		t.Parallel()

		fn := func(_ context.Context) error {
			return errors.New("always fails")
		}

		retryStrategyCalls := []int{}
		observer.UseConfig(ObserverConfig{
			Timeout:    time.Second,
			MaxRetries: 2,
			RetryStrategy: func(retryAttempt int) time.Duration {
				retryStrategyCalls = append(retryStrategyCalls, retryAttempt)
				return 1 * time.Millisecond
			},
		})

		err := observer.RunFunc(fn)
		if err == nil {
			t.Errorf("Expected error, got nil")
		}

		if len(retryStrategyCalls) != 2 {
			t.Fatalf("Expected 1 retry strategy call, got %d", len(retryStrategyCalls))
		}
		if retryStrategyCalls[0] != 1 && retryStrategyCalls[1] != 2 {
			t.Errorf(
				"Expected retry strategy called with args: 1 and 2",
			)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Timeouts:  0,
			Panics:    0,
			Retries:   2,
		})
	})
}

func TestObserve_InFlightMetrics(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)

	observer.UseConfig(ObserverConfig{
		Timeout: time.Second,
	})

	t.Run("concurrent in-flight tracking", func(t *testing.T) {
		t.Parallel()

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
					fn: func(_ context.Context) error {
						<-startBarrier
						time.Sleep(activeDuration)

						return nil
					},
				}

				err := observer.RunFunc(task.Execute)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}()
		}
		// Wait for all tasks to start
		time.Sleep(10 * time.Millisecond)

		// Verify that we have 10 tasks in flight
		inFlightCount := testutil.ToFloat64(observer.metrics.inFlight)
		if inFlightCount != numTasks {
			t.Errorf("Expected InFlight=%d when all tasks active, got %f", numTasks, inFlightCount)
		}

		// Release all tasks and wait for them to complete
		close(startBarrier)
		wg.Wait()

		// Verify that in-flight count returns to 0
		finalInFlight := testutil.ToFloat64(observer.metrics.inFlight)
		if finalInFlight != 0 {
			t.Errorf("Expected InFlight=0 after all tasks complete, got %f", finalInFlight)
		}
		if got := testutil.ToFloat64(observer.metrics.successes); got != numTasks {
			t.Errorf("Expected Successes=%d, got %f", numTasks, got)
		}

		Verify(t, observer, metricsCounts{
			Successes: float64(numTasks),
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})
}

func TestObserve_Concurrent(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)

	t.Run("concurrent task execution", func(t *testing.T) {
		t.Parallel()

		var wg sync.WaitGroup
		numGoroutines := 10
		wg.Add(numGoroutines)

		observer.UseConfig(ObserverConfig{
			Timeout: time.Second,
		})

		executed := int32(0)
		task := &testTask{
			fn: func(_ context.Context) error {
				defer wg.Done()
				atomic.AddInt32(&executed, 1)
				return nil
			},
		}

		for range numGoroutines {
			go func() {
				_ = observer.RunFunc(task.Execute)
			}()
		}

		wg.Wait()

		if got := atomic.LoadInt32(&executed); got != int32(numGoroutines) {
			t.Errorf("Expected %d executions, got %d", numGoroutines, got)
		}

		// Verify all tasks were recorded as successes
		Verify(t, observer, metricsCounts{
			Successes: float64(numGoroutines),
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})
}

func TestObserve_MetricsRecording(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name     string
		cfg      ObserverConfig
		task     *testTask
		wantErr  bool
		validate func(t *testing.T, observer *Observer)
		timesRun int
	}{
		{
			name: "successful task",
			task: &testTask{
				fn: func(_ context.Context) error {
					return nil
				},
			},
			wantErr: false,
			validate: func(t *testing.T, observer *Observer) {
				Verify(t, observer, metricsCounts{
					Successes: 1,
					Failures:  0,
					Errors:    0,
					Timeouts:  0,
					Panics:    0,
					Retries:   0,
				})
			},
			timesRun: 1,
		},
		{
			name: "failed task",
			task: &testTask{
				fn: func(_ context.Context) error {
					return errors.New("task failed")
				},
			},
			wantErr: true,
			validate: func(t *testing.T, observer *Observer) {
				Verify(t, observer, metricsCounts{
					Successes: 0,
					Failures:  1,
					Errors:    1,
					Timeouts:  0,
					Panics:    0,
					Retries:   0,
				})
			},
			timesRun: 1,
		},
		{
			name: "task with panic (recovered)",
			task: &testTask{
				fn: func(_ context.Context) error {
					panic("test panic")
				},
			},
			wantErr: true,
			validate: func(t *testing.T, observer *Observer) {
				Verify(t, observer, metricsCounts{
					Successes: 0,
					Failures:  1,
					Errors:    1,
					Timeouts:  0,
					Panics:    1,
					Retries:   0,
				})
			},
			timesRun: 1,
		},
		{
			name: "Multiple retries task with error returned",
			cfg: ObserverConfig{
				MaxRetries: 3,
			},
			task: &testTask{
				fn: func(_ context.Context) error {
					return errors.New("test error")
				},
			},
			wantErr: true,
			validate: func(t *testing.T, observer *Observer) {
				time.Sleep(10 * time.Millisecond)
				Verify(t, observer, metricsCounts{
					Successes: 0,
					Failures:  1,
					Errors:    4,
					Timeouts:  0,
					Panics:    0,
					Retries:   3,
				})
			},
			timesRun: 4,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			t.Logf("Running scenario: %s", scenario.name)

			observer := NewObserver(
				[]float64{1, 2, 3},
			)

			observer.UseConfig(scenario.cfg)
			registry := prometheus.NewRegistry()
			observer.MustRegister(registry)

			err := observer.RunFunc(scenario.task.Execute)
			if scenario.wantErr && err == nil {
				t.Error("Expected error but got none")
			}
			if !scenario.wantErr && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}

			scenario.validate(t, observer)

			families, err := registry.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			var histogramSampleCount uint64
			for _, family := range families {
				if *family.Name == "sentinel_durations_seconds" {
					if len(family.Metric) > 0 && family.Metric[0].Histogram != nil {
						histogramSampleCount = *family.Metric[0].Histogram.SampleCount
						break
					}
				}
			}

			if histogramSampleCount < uint64(scenario.timesRun) {
				t.Errorf("Test %s: Expected at least %d duration samples, got %d", scenario.name, scenario.timesRun, histogramSampleCount)
			}
		})
	}
}

func TestObserve_ContextTimeout(t *testing.T) {
	// t.Parallel()

	observer := NewObserver(nil)
	observer.UseConfig(ObserverConfig{
		Timeout:    20 * time.Millisecond,
		MaxRetries: 0,
	})

	fn := func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			return nil
		}
	}

	start := time.Now()
	err := observer.RunFunc(fn)
	duration := time.Since(start)

	t.Log("Should timeout quickly")
	if duration > 50*time.Millisecond {
		t.Errorf("Expected task to timeout quickly, took %v", duration)
	}

	t.Log("Should return deadline exceeded error")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	Verify(t, observer, metricsCounts{
		Successes: 0,
		Failures:  1,
		Errors:    1,
		Timeouts:  1,
		Panics:    0,
		Retries:   0,
	})
}

func TestMultipleObservers(t *testing.T) {
	t.Parallel()

	t.Log("Create observer1")
	observer1 := NewObserver(
		[]float64{0.01, 0.1, 1, 10, 100},
		WithNamespace("test"),
		WithSubsystem("observer1"),
		WithDescription("test operations"),
	)

	t.Log("Create observer2")
	observer2 := NewObserver(
		[]float64{0.01, 0.1, 1, 10, 100},
		WithNamespace("test"),
		WithSubsystem("observer2"),
		WithDescription("test operations"),
	)

	t.Log("Create registry")
	registry := prometheus.NewRegistry()
	observer1.MustRegister(registry)
	observer2.MustRegister(registry)

	t.Log("Run 17 tasks on observer1")
	for range 17 {
		_ = observer1.Run(func() error {
			return nil
		})
	}

	t.Log("Run 23 tasks on observer2")
	for range 23 {
		_ = observer2.Run(func() error {
			return nil
		})
	}

	t.Log("Fail 16 tasks on observer1")
	for range 16 {
		_ = observer1.Run(func() error {
			err := errors.New("test error")
			return err
		})
	}

	t.Log("Fail 14 tasks on observer2")
	for range 14 {
		_ = observer2.Run(func() error {
			err := errors.New("test error")
			return err
		})
	}

	Verify(t, observer1, metricsCounts{
		Successes: 17,
		Failures:  16,
		Errors:    16,
		Timeouts:  0,
		Panics:    0,
		Retries:   0,
	})

	Verify(t, observer2, metricsCounts{
		Successes: 23,
		Failures:  14,
		Errors:    14,
		Timeouts:  0,
		Panics:    0,
		Retries:   0,
	})
}

func TestObserver_TestPanicHandling(t *testing.T) {
	t.Parallel()

	t.Run("with panic recovery enabled (default)", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		fn := func(_ context.Context) error {
			panic("test panic")
		}

		err := observer.RunFunc(fn)
		if _, ok := IsPanicError(err); !ok {
			t.Errorf("Expected panic error, got %v", err)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    1,
			Retries:   0,
		})
	})

	t.Run("with panic recovery disabled via ObserverOption", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.DisablePanicRecovery(true)

		fn := func(_ context.Context) error {
			panic("test panic")
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic to propagate when DisablePanicRecovery(true)")
			} else {
				Verify(t, observer, metricsCounts{
					Successes: 0,
					Failures:  1,
					Errors:    1,
					Timeouts:  0,
					Panics:    1,
					Retries:   0,
				})
			}
		}()

		err := observer.RunFunc(fn)
		if _, ok := IsPanicError(err); !ok {
			t.Errorf("Expected panic error, got %v", err)
		}
	})
}

// failingTask is a test implementation of the Task interface.
type failingTask struct {
	attemptCount     int // do not touch
	shouldPanic      bool
	shouldSucceed    bool
	successOnAttempt int
	panicOnAttempt   int
}

func (t *failingTask) Execute(_ context.Context) error {
	t.attemptCount++
	if t.shouldPanic && t.attemptCount == t.panicOnAttempt {
		panic("task panic")
	}
	if t.shouldSucceed && t.attemptCount >= t.successOnAttempt {
		return nil
	}
	return errors.New("task failed")
}

func TestShortOnPanicRetryBreaker(t *testing.T) {
	t.Parallel()

	t.Run("allows retries on regular errors", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:      time.Second,
			MaxRetries:   3,
			RetryBreaker: circuit.OnPanic(),
		})

		task := &failingTask{
			shouldSucceed:    true,
			successOnAttempt: 3,
		}

		err := observer.RunFunc(task.Execute)
		if err != nil {
			t.Errorf("Expected no error after successful retry, got %v", err)
		}

		if task.attemptCount != 3 {
			t.Errorf("Expected 3 attempts, got %d", task.attemptCount)
		}

		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    2,
			Timeouts:  0,
			Panics:    0,
			Retries:   2,
		})
	})

	t.Run("stops retries immediately on panic", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:      time.Second,
			MaxRetries:   5,
			RetryBreaker: circuit.OnPanic(),
		})

		task := &failingTask{
			shouldPanic:    true,
			panicOnAttempt: 1,
		}

		err := observer.RunFunc(task.Execute)
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		var panicErr RecoveredPanic
		if !errors.As(err, &panicErr) {
			t.Errorf("Expected ErrRecoveredPanic, got %v", err)
		}

		if task.attemptCount != 1 {
			t.Errorf("Expected 1 attempts (circuit breaker stopped after panic on initial attempt), got %d", task.attemptCount)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    1,
			Retries:   0,
		})
	})

	t.Run("stops retries on panic during retry attempt", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:      time.Second,
			MaxRetries:   5,
			RetryBreaker: circuit.OnPanic(),
		})

		task := &failingTask{
			shouldPanic:    true,
			panicOnAttempt: 3,
		}

		err := observer.RunFunc(task.Execute)
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		var panicErr RecoveredPanic
		if !errors.As(err, &panicErr) {
			t.Errorf("Expected RecoveredPanic, got %v", err)
		}

		if task.attemptCount != 3 {
			t.Errorf("Expected 3 attempts (circuit breaker stopped after panic on retry), got %d", task.attemptCount)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Timeouts:  0,
			Panics:    1,
			Retries:   2,
		})
	})

	t.Run("continues retrying regular errors until max retries", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:      time.Second,
			MaxRetries:   3,
			RetryBreaker: circuit.OnPanic(),
		})

		task := &failingTask{
			shouldSucceed: false,
		}

		err := observer.RunFunc(task.Execute)
		if err == nil {
			t.Errorf("Expected error after all retries exhausted, got nil")
		}

		if task.attemptCount != 4 {
			t.Errorf("Expected 4 attempts (all retries exhausted), got %d", task.attemptCount)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    4,
			Timeouts:  0,
			Panics:    0,
			Retries:   3,
		})
	})
}

func TestRetryBreaker_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil circuit breaker behaves like DefaultRetryBreaker", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:      time.Second,
			MaxRetries:   2,
			RetryBreaker: nil,
		})

		task := &failingTask{
			shouldPanic:    true,
			panicOnAttempt: 3,
		}

		err := observer.RunFunc(task.Execute)
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		if task.attemptCount != 3 {
			t.Errorf("Expected 3 attempts with nil circuit breaker, got %d", task.attemptCount)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Timeouts:  0,
			Panics:    1,
			Retries:   2,
		})
	})

	t.Run("circuit breaker with custom error type", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)

		observer.UseConfig(ObserverConfig{
			Timeout:    time.Second,
			MaxRetries: 2,
			RetryBreaker: func(err error) bool {
				return errors.Is(err, RecoveredPanic{})
			},
		})

		task := &failingTask{
			shouldPanic:    true,
			panicOnAttempt: 1, // panic on first attempt
		}

		err := observer.RunFunc(task.Execute)
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Timeouts:  0,
			Panics:    1,
			Retries:   2,
		})
	})
}

func TestObserver_Describe(t *testing.T) {
	t.Parallel()

	t.Run("describes all metrics without duration buckets", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		ch := make(chan *prometheus.Desc, 10)

		go func() {
			observer.Describe(ch)
			close(ch)
		}()

		descs := make(map[string]*prometheus.Desc)
		for desc := range ch {
			if desc != nil {
				descs[desc.String()] = desc
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
		}

		if len(descs) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metric descriptions, got %d", len(expectedMetrics), len(descs))
		}

		// Verify that descriptions contain expected metric names
		descStrings := make([]string, 0, len(descs))
		for descStr := range descs {
			descStrings = append(descStrings, descStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, descStr := range descStrings {
				if strings.Contains(descStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find description for metric %s", expectedName)
			}
		}
	})

	t.Run("describes all metrics with duration buckets", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver([]float64{0.1, 0.5, 1, 2, 5})
		ch := make(chan *prometheus.Desc, 10)

		go func() {
			observer.Describe(ch)
			close(ch)
		}()

		descs := make(map[string]*prometheus.Desc)
		for desc := range ch {
			if desc != nil {
				descs[desc.String()] = desc
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
			"sentinel_durations_seconds",
		}

		if len(descs) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metric descriptions, got %d", len(expectedMetrics), len(descs))
		}

		descStrings := make([]string, 0, len(descs))
		for descStr := range descs {
			descStrings = append(descStrings, descStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, descStr := range descStrings {
				if strings.Contains(descStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find description for metric %s", expectedName)
			}
		}
	})
}

func TestObserver_Collect(t *testing.T) {
	t.Parallel()

	t.Run("collects all metrics without duration buckets", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		ch := make(chan prometheus.Metric, 10)

		go func() {
			observer.Collect(ch)
			close(ch)
		}()

		metrics := make(map[string]prometheus.Metric)
		for metric := range ch {
			if metric != nil {
				desc := metric.Desc()
				metrics[desc.String()] = metric
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
		}

		if len(metrics) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metrics, got %d", len(expectedMetrics), len(metrics))
		}

		metricStrings := make([]string, 0, len(metrics))
		for metricStr := range metrics {
			metricStrings = append(metricStrings, metricStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, metricStr := range metricStrings {
				if strings.Contains(metricStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find metric %s", expectedName)
			}
		}
	})

	t.Run("collects all metrics with duration buckets", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver([]float64{0.1, 0.5, 1, 2, 5})
		ch := make(chan prometheus.Metric, 10)

		go func() {
			observer.Collect(ch)
			close(ch)
		}()

		metrics := make(map[string]prometheus.Metric)
		for metric := range ch {
			if metric != nil {
				desc := metric.Desc()
				metrics[desc.String()] = metric
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
			"sentinel_durations_seconds",
		}

		if len(metrics) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metrics, got %d", len(expectedMetrics), len(metrics))
		}

		metricStrings := make([]string, 0, len(metrics))
		for metricStr := range metrics {
			metricStrings = append(metricStrings, metricStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, metricStr := range metricStrings {
				if strings.Contains(metricStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find metric %s", expectedName)
			}
		}
	})

	t.Run("collects metrics after task execution", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver([]float64{0.1, 0.5, 1, 2, 5})
		_ = observer.Run(func() error {
			return nil
		})

		ch := make(chan prometheus.Metric, 10)
		go func() {
			observer.Collect(ch)
			close(ch)
		}()

		metrics := make(map[string]prometheus.Metric)
		for metric := range ch {
			if metric != nil {
				desc := metric.Desc()
				metrics[desc.String()] = metric
			}
		}

		if len(metrics) == 0 {
			t.Error("Expected to collect at least one metric after task execution")
		}

		// Verify that success metric has a value > 0
		for metricStr, metric := range metrics {
			if strings.Contains(metricStr, "sentinel_success_total") {
				m := metric
				dtoMetric := &dto.Metric{}
				if err := m.Write(dtoMetric); err == nil {
					if dtoMetric.Counter != nil && dtoMetric.Counter.Value != nil && *dtoMetric.Counter.Value > 0 {
						return // Found success metric with value > 0
					}
				}
			}
		}
	})
}

func TestHandlerPanicRecovery(t *testing.T) {
	t.Parallel()

	t.Run("retry strategy panic recovery", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int // nil pointer that will cause panic when dereferenced

		observer.UseConfig(ObserverConfig{
			MaxRetries: 1,
			RetryStrategy: retry.WaitFunc(func(_ int) time.Duration {
				// This will panic due to nil pointer dereference
				return time.Duration(*i)
			}),
		})

		// Should not panic, but should handle gracefully
		err := observer.Run(func() error {
			return errors.New("task failed")
		})

		// Should have attempted retry (with 0 wait time due to panic recovery default)
		if err == nil {
			t.Error("Expected error after retries, got nil")
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    2, // Initial error + retry error
			Timeouts:  0,
			Panics:    0, // Handler panics are recovered, not task panics
			Retries:   1,
		})
	})

	t.Run("retry breaker panic recovery", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int // nil pointer that will cause panic when dereferenced

		observer.UseConfig(ObserverConfig{
			MaxRetries: 3,
			RetryBreaker: circuit.Breaker(func(_ error) bool {
				// This will panic due to nil pointer dereference
				_ = *i
				return false
			}),
		})

		// Should not panic, but should stop retries due to panic recovery default (true)
		err := observer.Run(func() error {
			return errors.New("task failed")
		})

		// Should have stopped after first attempt due to breaker panic recovery
		if err == nil {
			t.Error("Expected error, got nil")
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1, // Only initial error, breaker panic stops retries
			Timeouts:  0,
			Panics:    0,
			Retries:   0, // No retries because breaker panic defaults to stopping
		})
	})

	t.Run("request control panic recovery", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int // nil pointer that will cause panic when dereferenced

		executed := false
		observer.UseConfig(ObserverConfig{
			Control: circuit.Control(func(_ circuit.ExecutionPhase) bool {
				// This will panic due to nil pointer dereference
				_ = *i
				return false
			}),
		})

		// Should not panic; control panic defaults to allowing execution (false)
		err := observer.Run(func() error {
			executed = true
			return nil
		})

		// Task should execute and succeed; panics_total increments for the callback panic
		if err != nil {
			t.Errorf("Expected nil error when Control panics (allow execution), got %v", err)
		}
		if !executed {
			t.Error("Task should have executed when Control panics")
		}

		Verify(t, observer, metricsCounts{
			Successes: 1, // Task executed and succeeded
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    1, // Control callback panic recorded
			Retries:   0,
		})
	})

	t.Run("in-flight control panic recovery", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int // nil pointer that will cause panic when dereferenced

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			Control: circuit.Control(func(_ circuit.ExecutionPhase) bool {
				// This will panic due to nil pointer dereference
				_ = *i
				return false
			}),
		})

		// Should not panic; control panic defaults to allowing execution (false) so all retries proceed
		err := observer.Run(func() error {
			return errors.New("task failed")
		})

		// All 3 attempts run (initial + 2 retries), exhausting MaxRetries
		if err == nil {
			t.Error("Expected error, got nil")
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3, // All 3 attempts failed (initial + 2 retries)
			Timeouts:  0,
			Panics:    3, // One panic per control check (NewRequest + 2 PhaseRetry checks)
			Retries:   2, // Both retries proceeded due to control panic allowing execution
		})
	})
}

// TestHandlerPanicRecovery_Comprehensive provides comprehensive test cases
// to validate panic recovery behavior for all handler types.
func TestHandlerPanicRecovery_Comprehensive(t *testing.T) {
	t.Parallel()

	t.Run("retry strategy panic on first retry attempt", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			RetryStrategy: retry.WaitFunc(func(retryCount int) time.Duration {
				if retryCount == 1 {
					// Panic on first retry attempt
					return time.Duration(*i)
				}
				return 100 * time.Millisecond
			}),
		})

		attempts := 0
		err := observer.Run(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("task failed")
			}
			return nil
		})

		// Should succeed after retries (panic recovery defaults to 0 wait)
		if err != nil {
			t.Errorf("Expected success after retries, got %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}

		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    2,
			Timeouts:  0,
			Panics:    0,
			Retries:   2,
		})
	})

	t.Run("retry strategy panic on all retry attempts", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			RetryStrategy: retry.WaitFunc(func(_ int) time.Duration {
				// Always panic
				return time.Duration(*i)
			}),
		})

		attempts := 0
		err := observer.Run(func() error {
			attempts++
			return errors.New("task failed")
		})

		// Should fail after all retries exhausted
		if err == nil {
			t.Error("Expected error after retries exhausted, got nil")
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts (initial + 2 retries), got %d", attempts)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Timeouts:  0,
			Panics:    0,
			Retries:   2,
		})
	})

	t.Run("retry breaker panic with different error types", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 3,
			RetryBreaker: circuit.Breaker(func(_ error) bool {
				// Panic when checking error
				_ = *i
				return false
			}),
		})

		err := observer.Run(func() error {
			return errors.New("task failed")
		})

		// Should stop after first attempt due to breaker panic
		if err == nil {
			t.Error("Expected error, got nil")
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("request control panic allows task execution", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int
		taskExecuted := false

		observer.UseConfig(ObserverConfig{
			Control: circuit.Control(func(_ circuit.ExecutionPhase) bool {
				// Panic before returning
				_ = *i
				return false
			}),
		})

		err := observer.Run(func() error {
			taskExecuted = true
			return nil
		})

		// Control panic defaults to allowing execution (false = allow)
		if !taskExecuted {
			t.Error("Task should have executed when control panics (allow execution)")
		}

		if err != nil {
			t.Errorf("Expected nil error when Control panics (allow execution), got %v", err)
		}

		// Control panic allows execution; panics_total incremented for callback panic
		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    1, // Control callback panic recorded
			Retries:   0,
		})
	})

	t.Run("in-flight control panic on second retry attempt allows further retries", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int
		callCount := 0

		observer.UseConfig(ObserverConfig{
			MaxRetries: 3,
			Control: circuit.Control(func(_ circuit.ExecutionPhase) bool {
				callCount++
				if callCount == 3 {
					// Panic on third check; control panic defaults to allowing execution
					_ = *i
				}
				return false
			}),
		})

		attempts := 0
		err := observer.Run(func() error {
			attempts++
			return errors.New("task failed")
		})

		// Control panic allows retries to continue; all 4 attempts run (initial + 3 retries)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		if attempts != 4 {
			t.Errorf("Expected 4 attempts (initial + 3 retries), got %d", attempts)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    4,
			Timeouts:  0,
			Panics:    1, // Third control check panicked
			Retries:   3,
		})
	})

	t.Run("multiple handlers with panics - retry strategy and breaker", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			RetryStrategy: retry.WaitFunc(func(_ int) time.Duration {
				return time.Duration(*i) // Panic
			}),
			RetryBreaker: circuit.Breaker(func(_ error) bool {
				_ = *i // Panic
				return false
			}),
		})

		attempts := 0
		err := observer.Run(func() error {
			attempts++
			return errors.New("task failed")
		})

		// Breaker panic should stop retries first (checked before strategy)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		if attempts != 1 {
			t.Errorf("Expected 1 attempt (breaker panic stops retries), got %d", attempts)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1, // Failed to execute task
			Errors:    1, // Control handler causes error
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("handler panic does not affect successful task", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			RetryStrategy: retry.WaitFunc(func(_ int) time.Duration {
				return time.Duration(*i) // Panic, but task succeeds so never called
			}),
		})

		err := observer.Run(func() error {
			return nil // Success on first attempt
		})

		// Should succeed without issues
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("retry strategy panic with successful retry", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			RetryStrategy: retry.WaitFunc(func(retryCount int) time.Duration {
				if retryCount == 1 {
					return time.Duration(*i) // Panic on first retry
				}
				return 10 * time.Millisecond
			}),
		})

		attempts := 0
		err := observer.Run(func() error {
			attempts++
			if attempts == 1 {
				return errors.New("task failed")
			}
			return nil // Success on retry
		})

		// Should succeed after retry (panic recovery allows retry with 0 wait)
		if err != nil {
			t.Errorf("Expected success after retry, got %v", err)
		}

		if attempts != 2 {
			t.Errorf("Expected 2 attempts, got %d", attempts)
		}

		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   1,
		})
	})

	t.Run("request control panic in RunFunc allows execution", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int
		taskExecuted := false

		observer.UseConfig(ObserverConfig{
			Control: circuit.Control(func(_ circuit.ExecutionPhase) bool {
				_ = *i // Panic
				return false
			}),
		})

		err := observer.RunFunc(func(_ context.Context) error {
			taskExecuted = true
			return nil
		})

		// Control panic defaults to allowing execution
		if !taskExecuted {
			t.Error("Task should have executed when control panics (allow execution)")
		}

		if err != nil {
			t.Errorf("Expected nil error when Control panics, got %v", err)
		}
	})

	t.Run("handler panics do not propagate to caller", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 1,
			RetryStrategy: retry.WaitFunc(func(_ int) time.Duration {
				return time.Duration(*i) // Panic
			}),
		})

		// Should not panic - recovery should handle it
		panicked := false
		func() {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
				}
			}()
			_ = observer.Run(func() error {
				return errors.New("task failed")
			})
		}()

		if panicked {
			t.Error("Handler panic should not propagate to caller")
		}
	})

	t.Run("retry breaker panic with nil error", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			RetryBreaker: circuit.Breaker(func(_ error) bool {
				// Panic even with nil error (shouldn't happen but test edge case)
				_ = *i
				return false
			}),
		})

		// Task succeeds, breaker shouldn't be called, but if it is, panic recovery handles it
		err := observer.Run(func() error {
			return nil
		})

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("concurrent execution with handler panics", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		observer.UseConfig(ObserverConfig{
			MaxRetries: 1,
			RetryStrategy: retry.WaitFunc(func(_ int) time.Duration {
				return time.Duration(*i) // Panic
			}),
		})

		var wg sync.WaitGroup
		taskErrors := make([]error, 10)

		// Run 10 concurrent tasks
		for j := 0; j < 10; j++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				taskErrors[idx] = observer.Run(func() error {
					return errors.New("task failed")
				})
			}(j)
		}

		wg.Wait()

		// All should complete without panicking
		for idx, err := range taskErrors {
			if err == nil {
				t.Errorf("Goroutine %d: Expected error, got nil", idx)
			}
		}

		// Verify metrics
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  10,
			Errors:    20, // 10 initial + 10 retries
			Timeouts:  0,
			Panics:    0,
			Retries:   10,
		})
	})

	t.Run("retry strategy panic with exponential backoff wrapper", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int

		// Wrap exponential backoff with a panicking function
		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			RetryStrategy: retry.WaitFunc(func(retryCount int) time.Duration {
				if retryCount == 1 {
					return time.Duration(*i) // Panic
				}
				return retry.Exponential(10 * time.Millisecond)(retryCount)
			}),
		})

		attempts := 0
		err := observer.Run(func() error {
			attempts++
			if attempts < 3 {
				return errors.New("task failed")
			}
			return nil
		})

		// Should succeed after retries
		if err != nil {
			t.Errorf("Expected success, got %v", err)
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts, got %d", attempts)
		}
	})

	t.Run("in-flight control panic after successful retry check allows further retries", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		var i *int
		checkCount := 0

		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			Control: circuit.Control(func(_ circuit.ExecutionPhase) bool {
				checkCount++
				if checkCount == 3 {
					// Panic on third check; control panic defaults to allowing execution
					_ = *i
				}
				return false // Allow retries initially
			}),
		})

		attempts := 0
		err := observer.Run(func() error {
			attempts++
			return errors.New("task failed")
		})

		// Control panic allows retries; all 3 attempts run (initial + 2 retries)
		if err == nil {
			t.Error("Expected error, got nil")
		}

		if attempts != 3 {
			t.Errorf("Expected 3 attempts (initial + 2 retries), got %d", attempts)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3, // All 3 attempts failed
			Timeouts:  0,
			Panics:    1, // Third control check panicked
			Retries:   2, // Both retries proceeded
		})
	})
}

func TestObserver_PendingAndInFlightMetrics(t *testing.T) {
	t.Parallel()

	t.Run("immediate acquisition does not increment pending", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			MaxConcurrency: 10, // Large limit, should acquire immediately
		})

		err := observer.RunFunc(func(_ context.Context) error {
			return nil
		})
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		pending := testutil.ToFloat64(observer.metrics.pending)
		if pending != 0 {
			t.Errorf("Expected Pending=0 for immediate acquisition, got %f", pending)
		}

		inFlight := testutil.ToFloat64(observer.metrics.inFlight)
		if inFlight != 0 {
			t.Errorf("Expected InFlight=0 after completion, got %f", inFlight)
		}
	})

	t.Run("blocking acquisition increments pending", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			MaxConcurrency: 2, // Small limit to force blocking
		})

		// Start two tasks that will hold the limiter slots
		blocker1 := make(chan struct{})
		blocker2 := make(chan struct{})
		var wg sync.WaitGroup

		wg.Add(2)
		// Start two tasks that block
		go func() {
			defer wg.Done()
			_ = observer.RunFunc(func(_ context.Context) error {
				<-blocker1
				return nil
			})
		}()
		go func() {
			defer wg.Done()
			_ = observer.RunFunc(func(_ context.Context) error {
				<-blocker2
				return nil
			})
		}()

		// Wait for both to acquire slots
		time.Sleep(50 * time.Millisecond)

		// Now start a third task that should be pending
		pendingDone := make(chan struct{})
		go func() {
			defer close(pendingDone)
			_ = observer.RunFunc(func(_ context.Context) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			})
		}()

		// Give it time to start and check pending from outside
		time.Sleep(30 * time.Millisecond)
		pendingCount := testutil.ToFloat64(observer.metrics.pending)

		// Verify pending was incremented (should be 1 while waiting)
		if pendingCount == 0 {
			t.Errorf("Expected Pending>0 while waiting for limiter slot, got %f", pendingCount)
		}

		// Release blockers
		close(blocker1)
		close(blocker2)
		wg.Wait()
		<-pendingDone

		// Verify pending returns to 0
		finalPending := testutil.ToFloat64(observer.metrics.pending)
		if finalPending != 0 {
			t.Errorf("Expected Pending=0 after completion, got %f", finalPending)
		}
	})

	t.Run("concurrent tasks with limiter", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		const maxConcurrency = 3
		const numTasks = 10
		observer.UseConfig(ObserverConfig{
			MaxConcurrency: maxConcurrency,
		})

		startBarrier := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(numTasks)

		// Track metrics during execution
		var maxPending, maxInFlight float64
		var mu sync.Mutex

		// Start all tasks concurrently
		for i := 0; i < numTasks; i++ {
			go func() {
				defer wg.Done()
				_ = observer.RunFunc(func(_ context.Context) error {
					<-startBarrier
					// Check metrics while executing
					mu.Lock()
					pending := testutil.ToFloat64(observer.metrics.pending)
					inFlight := testutil.ToFloat64(observer.metrics.inFlight)
					if pending > maxPending {
						maxPending = pending
					}
					if inFlight > maxInFlight {
						maxInFlight = inFlight
					}
					mu.Unlock()
					time.Sleep(20 * time.Millisecond)
					return nil
				})
			}()
		}

		// Wait for tasks to start and acquire limiter slots
		time.Sleep(50 * time.Millisecond)

		// Release all tasks
		close(startBarrier)
		wg.Wait()

		// Verify final state
		finalPending := testutil.ToFloat64(observer.metrics.pending)
		if finalPending != 0 {
			t.Errorf("Expected Pending=0 after all tasks complete, got %f", finalPending)
		}

		finalInFlight := testutil.ToFloat64(observer.metrics.inFlight)
		if finalInFlight != 0 {
			t.Errorf("Expected InFlight=0 after all tasks complete, got %f", finalInFlight)
		}

		// Verify we had some pending at some point (since numTasks > maxConcurrency)
		if maxPending == 0 {
			t.Error("Expected MaxPending>0 when numTasks > maxConcurrency, got 0")
		}

		// Verify inFlight never exceeded maxConcurrency
		if maxInFlight > float64(maxConcurrency) {
			t.Errorf("Expected MaxInFlight<=%d, got %f", maxConcurrency, maxInFlight)
		}

		// Verify all tasks succeeded
		successes := testutil.ToFloat64(observer.metrics.successes)
		if successes != numTasks {
			t.Errorf("Expected Successes=%d, got %f", numTasks, successes)
		}
	})

	t.Run("pending decreases when slot becomes available", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			MaxConcurrency: 1, // Only one slot
		})

		// First task holds the slot
		task1Done := make(chan struct{})
		go func() {
			defer close(task1Done)
			_ = observer.RunFunc(func(_ context.Context) error {
				time.Sleep(100 * time.Millisecond)
				return nil
			})
		}()

		// Wait for first task to acquire
		time.Sleep(20 * time.Millisecond)

		// Second task should be pending
		task2Done := make(chan struct{})
		go func() {
			defer close(task2Done)
			_ = observer.RunFunc(func(_ context.Context) error {
				time.Sleep(10 * time.Millisecond)
				return nil
			})
		}()

		// Wait for task2 to start and be pending
		time.Sleep(30 * time.Millisecond)

		// Check pending from outside while task2 is waiting
		pendingBefore := testutil.ToFloat64(observer.metrics.pending)
		if pendingBefore == 0 {
			t.Errorf("Expected PendingBefore>0 while task2 waits, got %f", pendingBefore)
		}

		// Wait for task1 to finish, which should allow task2 to acquire
		<-task1Done

		// Give task2 time to acquire and check pending again
		time.Sleep(20 * time.Millisecond)
		pendingAfter := testutil.ToFloat64(observer.metrics.pending)

		// Wait for task2 to complete
		<-task2Done

		// Verify pending decreased after acquisition
		if pendingAfter != 0 {
			t.Errorf("Expected PendingAfter=0 after acquisition, got %f", pendingAfter)
		}

		// Final pending should be 0
		finalPending := testutil.ToFloat64(observer.metrics.pending)
		if finalPending != 0 {
			t.Errorf("Expected FinalPending=0, got %f", finalPending)
		}
	})

	t.Run("inFlight tracks executing tasks correctly", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			MaxConcurrency: 5,
		})

		const numTasks = 8
		startBarrier := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(numTasks)

		// Start tasks
		for i := 0; i < numTasks; i++ {
			go func() {
				defer wg.Done()
				_ = observer.RunFunc(func(_ context.Context) error {
					<-startBarrier
					time.Sleep(30 * time.Millisecond)
					return nil
				})
			}()
		}

		// Wait for tasks to start
		time.Sleep(50 * time.Millisecond)

		// Check inFlight - should be at most maxConcurrency
		inFlight := testutil.ToFloat64(observer.metrics.inFlight)
		if inFlight > 5 {
			t.Errorf("Expected InFlight<=5 (maxConcurrency), got %f", inFlight)
		}
		if inFlight == 0 {
			t.Error("Expected InFlight>0 when tasks are executing, got 0")
		}

		// Release and wait for completion
		close(startBarrier)
		wg.Wait()

		// Verify inFlight returns to 0
		finalInFlight := testutil.ToFloat64(observer.metrics.inFlight)
		if finalInFlight != 0 {
			t.Errorf("Expected InFlight=0 after completion, got %f", finalInFlight)
		}
	})

	t.Run("no limiter means no pending tracking", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			MaxConcurrency: 0, // No limiter
		})

		const numTasks = 10
		var wg sync.WaitGroup
		wg.Add(numTasks)

		// Start many concurrent tasks
		for i := 0; i < numTasks; i++ {
			go func() {
				defer wg.Done()
				_ = observer.RunFunc(func(_ context.Context) error {
					time.Sleep(10 * time.Millisecond)
					return nil
				})
			}()
		}

		wg.Wait()

		// Pending should always be 0 when no limiter
		pending := testutil.ToFloat64(observer.metrics.pending)
		if pending != 0 {
			t.Errorf("Expected Pending=0 when no limiter, got %f", pending)
		}

		// But inFlight should still track
		inFlight := testutil.ToFloat64(observer.metrics.inFlight)
		if inFlight != 0 {
			t.Errorf("Expected InFlight=0 after completion, got %f", inFlight)
		}
	})

	t.Run("pending and inFlight relationship", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{
			MaxConcurrency: 2,
		})

		// Start 2 tasks that hold slots
		blocker := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			_ = observer.RunFunc(func(_ context.Context) error {
				<-blocker
				return nil
			})
		}()
		go func() {
			defer wg.Done()
			_ = observer.RunFunc(func(_ context.Context) error {
				<-blocker
				return nil
			})
		}()

		// Wait for them to acquire
		time.Sleep(30 * time.Millisecond)

		// Start 3 more tasks (should have 2 inFlight, 3 pending)
		wg.Add(3)
		for i := 0; i < 3; i++ {
			go func() {
				defer wg.Done()
				_ = observer.RunFunc(func(_ context.Context) error {
					time.Sleep(20 * time.Millisecond)
					return nil
				})
			}()
		}

		// Wait a bit for them to start
		time.Sleep(30 * time.Millisecond)

		// Check metrics
		inFlight := testutil.ToFloat64(observer.metrics.inFlight)
		pending := testutil.ToFloat64(observer.metrics.pending)

		// Should have 2 inFlight (maxConcurrency)
		if inFlight != 2 {
			t.Errorf("Expected InFlight=2 (maxConcurrency), got %f", inFlight)
		}

		// Should have some pending (at least 3, possibly more if timing)
		if pending == 0 {
			t.Error("Expected Pending>0 when tasks are waiting, got 0")
		}

		// Release blockers
		close(blocker)
		wg.Wait()

		// Verify final state
		finalInFlight := testutil.ToFloat64(observer.metrics.inFlight)
		finalPending := testutil.ToFloat64(observer.metrics.pending)
		if finalInFlight != 0 {
			t.Errorf("Expected FinalInFlight=0, got %f", finalInFlight)
		}
		if finalPending != 0 {
			t.Errorf("Expected FinalPending=0, got %f", finalPending)
		}
	})
}

func TestSubmit(t *testing.T) {
	t.Run("executes all submitted tasks", func(t *testing.T) {
		observer := NewObserver(nil)
		var count atomic.Int64
		for i := 0; i < 5; i++ {
			observer.Submit(func() error {
				count.Add(1)
				return nil
			})
		}
		observer.pool.Wait()
		if got := count.Load(); got != 5 {
			t.Errorf("Expected 5 tasks to execute, got %d", got)
		}
	})

	t.Run("Wait blocks until all tasks complete", func(t *testing.T) {
		observer := NewObserver(nil)
		var count atomic.Int64
		blocker := make(chan struct{})
		for i := 0; i < 5; i++ {
			observer.Submit(func() error {
				<-blocker
				count.Add(1)
				return nil
			})
		}
		close(blocker)
		observer.pool.Wait()
		if got := count.Load(); got != 5 {
			t.Errorf("Expected 5 tasks after Wait, got %d", got)
		}
	})

	t.Run("records success metrics", func(t *testing.T) {
		observer := NewObserver(nil)
		for i := 0; i < 3; i++ {
			observer.Submit(func() error { return nil })
		}
		observer.pool.Wait()
		if got := testutil.ToFloat64(observer.metrics.successes); got != 3 {
			t.Errorf("Expected successes=3, got %f", got)
		}
	})

	t.Run("records error metrics", func(t *testing.T) {
		observer := NewObserver(nil)
		observer.Submit(func() error { return errors.New("fail") })
		observer.pool.Wait()
		if got := testutil.ToFloat64(observer.metrics.errors); got != 1 {
			t.Errorf("Expected errors=1, got %f", got)
		}
	})
}

func TestSubmitFunc(t *testing.T) {
	t.Run("fn receives a context", func(t *testing.T) {
		observer := NewObserver(nil)
		ctxReceived := make(chan context.Context, 1)
		observer.SubmitFunc(func(ctx context.Context) error {
			ctxReceived <- ctx
			return nil
		})
		observer.pool.Wait()
		select {
		case ctx := <-ctxReceived:
			if ctx == nil {
				t.Error("Expected non-nil context")
			}
		default:
			t.Error("fn was not called")
		}
	})

	t.Run("timeout fires for slow tasks", func(t *testing.T) {
		observer := NewObserver(nil)
		observer.UseConfig(ObserverConfig{Timeout: 50 * time.Millisecond})
		var gotErr atomic.Value
		observer.SubmitFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				gotErr.Store(ctx.Err())
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		})
		observer.pool.Wait()
		if err, ok := gotErr.Load().(error); !ok || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected DeadlineExceeded, got %v", gotErr.Load())
		}
	})
}

func TestSubmit_PanicPropagation(t *testing.T) {
	t.Run("panic in Submit task is recovered - process does not crash", func(t *testing.T) {
		observer := NewObserver(nil)
		observer.Submit(func() error {
			panic("test panic")
		})
		observer.pool.Wait()
		Verify(t, observer, metricsCounts{
			Panics:   1,
			Errors:   1,
			Failures: 1,
		})
	})

	t.Run("panics_total incremented for Submit panic", func(t *testing.T) {
		observer := NewObserver(nil)
		observer.Submit(func() error {
			panic("test panic")
		})
		observer.pool.Wait()
		if got := testutil.ToFloat64(observer.metrics.panics); got != 1 {
			t.Errorf("Expected panics=1, got %f", got)
		}
	})

	t.Run("mix of normal and panicking Submit tasks - normal tasks complete", func(t *testing.T) {
		observer := NewObserver(nil)
		var count atomic.Int64
		for i := 0; i < 3; i++ {
			observer.Submit(func() error {
				count.Add(1)
				return nil
			})
		}
		observer.Submit(func() error {
			panic("test panic")
		})
		observer.pool.Wait()
		if got := count.Load(); got != 3 {
			t.Errorf("Expected 3 normal tasks to complete, got %d", got)
		}
		Verify(t, observer, metricsCounts{
			Successes: 3,
			Panics:    1,
			Errors:    1,
			Failures:  1,
		})
	})

	t.Run("panic with DisablePanicRecovery - conc pool captures and re-panics on pool.Wait()", func(t *testing.T) {
		observer := NewObserver(nil)
		observer.DisablePanicRecovery(true)
		observer.Submit(func() error {
			panic("test panic")
		})
		var caught bool
		func() {
			defer func() {
				if r := recover(); r != nil {
					caught = true
				}
			}()
			observer.pool.Wait()
		}()
		if !caught {
			t.Error("Expected pool.Wait() to re-panic when DisablePanicRecovery is true")
		}
		// Metrics are recorded before the re-panic propagates
		Verify(t, observer, metricsCounts{
			Panics:   1,
			Errors:   1,
			Failures: 1,
		})
	})

	t.Run("RecoveredPanic type consistent between sync and async paths", func(t *testing.T) {
		// Sync path returns RecoveredPanic as an error
		syncObs := NewObserver(nil)
		syncErr := syncObs.Run(func() error {
			panic("test panic")
		})
		if _, ok := IsPanicError(syncErr); !ok {
			t.Error("Expected sync Run() to return RecoveredPanic error")
		}

		// Async path: panic internally produces RecoveredPanic but is discarded by Submit().
		// Metrics are still recorded consistently with the sync path.
		asyncObs := NewObserver(nil)
		asyncObs.Submit(func() error {
			panic("test panic")
		})
		asyncObs.pool.Wait()
		if got := testutil.ToFloat64(asyncObs.metrics.panics); got != 1 {
			t.Errorf("Async path: expected panics=1 (consistent with sync), got %f", got)
		}
		Verify(t, asyncObs, metricsCounts{
			Panics:   1,
			Errors:   1,
			Failures: 1,
		})
	})
}
