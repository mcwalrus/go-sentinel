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
	fn       func(ctx context.Context) error
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
		var observer *Observer = nil

		t.Run("Run", func(t *testing.T) {
			t.Parallel()
			defer expectPanic(t, "observer.Run")
			_ = observer.Run(func() error { return nil })
		})

		t.Run("RunFunc", func(t *testing.T) {
			t.Parallel()
			defer expectPanic(t, "observer.RunFunc")
			_ = observer.RunFunc(func(ctx context.Context) error { return nil })
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
			_ = observer.RunFunc(func(ctx context.Context) error { return nil })
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
		fn: func(ctx context.Context) error {
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
			fn: func(ctx context.Context) error {
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
			fn: func(ctx context.Context) error {
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
			fn: func(ctx context.Context) error {
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
			fn: func(ctx context.Context) error {
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
			fn: func(ctx context.Context) error {
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

func TestObserve_RetryStrategy(t *testing.T) {
	t.Parallel()

	observer := NewObserver(nil)

	t.Run("retry strategy is called with correct parameters", func(t *testing.T) {
		t.Parallel()

		fn := func(ctx context.Context) error {
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
					fn: func(ctx context.Context) error {
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
			fn: func(ctx context.Context) error {
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
				fn: func(ctx context.Context) error {
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
				fn: func(ctx context.Context) error {
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
				fn: func(ctx context.Context) error {
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
				fn: func(ctx context.Context) error {
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

		fn := func(ctx context.Context) error {
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

		fn := func(ctx context.Context) error {
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

	// t.Run("with panic recovery from forked inherited observer", func(t *testing.T) {
	// 	t.Parallel()

	// 	observer := NewObserver(nil)
	// 	observer.DisablePanicRecovery(true)
	// 	forkedObserver := observer.Fork()

	// 	fn := func(ctx context.Context) error {
	// 		panic("test panic")
	// 	}

	// 	defer func() {
	// 		if r := recover(); r == nil {
	// 			t.Error("Expected panic to propagate when DisablePanicRecovery(true)")
	// 		} else {
	// 			Verify(t, observer, metricsCounts{
	// 				Successes: 0,
	// 				Failures:  1,
	// 				Errors:    1,
	// 				Timeouts:  0,
	// 				Panics:    1,
	// 				Retries:   0,
	// 			})
	// 		}
	// 	}()

	// 	err := forkedObserver.RunFunc(fn)
	// 	if _, ok := IsPanicError(err); !ok {
	// 		t.Errorf("Expected panic error, got nil")
	// 	}
	// })

	// t.Run("with panic recovery from forked inherited observer maintained", func(t *testing.T) {
	// 	t.Parallel()

	// 	observer := NewObserver(nil)
	// 	observer.DisablePanicRecovery(true)
	// 	forkedObserver := observer.Fork()
	// 	observer.DisablePanicRecovery(false)

	// 	fn := func(ctx context.Context) error {
	// 		panic("test panic")
	// 	}

	// 	// closure to catch panic
	// 	func() {
	// 		t.Log("Forked observer maintains panic recovery disabled")
	// 		defer func() {
	// 			if r := recover(); r == nil {
	// 				t.Error("Expected panic to propagate when DisablePanicRecovery(true)")
	// 			} else {
	// 				Verify(t, observer, metricsCounts{
	// 					Successes: 0,
	// 					Failures:  1,
	// 					Errors:    1,
	// 					Timeouts:  0,
	// 					Panics:    1,
	// 					Retries:   0,
	// 				})
	// 			}
	// 		}()

	// 		err := forkedObserver.RunFunc(fn)
	// 		if _, ok := IsPanicError(err); !ok {
	// 			t.Errorf("Expected panic error, got nil")
	// 		}
	// 	}()

	// 	// closure to catch panic
	// 	func() {
	// 		t.Log("Base observer has panic recovery enabled")
	// 		defer func() {
	// 			if r := recover(); r != nil {
	// 				t.Error("Expected panic to not propagate when DisablePanicRecovery(false)")
	// 			}
	// 		}()

	// 		err := observer.RunFunc(fn)
	// 		if _, ok := IsPanicError(err); !ok {
	// 			t.Errorf("Expected panic error, got nil")
	// 		}

	// 		Verify(t, observer, metricsCounts{
	// 			Successes: 0,
	// 			Failures:  2,
	// 			Errors:    2,
	// 			Timeouts:  0,
	// 			Panics:    2,
	// 			Retries:   0,
	// 		})
	// 	}()
	// })
}

// failingTask is a test implementation of the Task interface.
type failingTask struct {
	attemptCount     int // do not touch
	shouldPanic      bool
	shouldSucceed    bool
	successOnAttempt int
	panicOnAttempt   int
}

func (t *failingTask) Execute(ctx context.Context) error {
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

		var panicErr *ErrRecoveredPanic
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

		var panicErr *ErrRecoveredPanic
		if !errors.As(err, &panicErr) {
			t.Errorf("Expected ErrRecoveredPanic, got %v", err)
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
				return errors.Is(err, &ErrRecoveredPanic{})
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
		observer.Run(func() error {
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
				var m prometheus.Metric = metric
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
