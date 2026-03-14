package sentinel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mcwalrus/go-sentinel/circuit"
	"github.com/mcwalrus/go-sentinel/retry"
)

func TestObserver_ControlAvoidsInitialExecution(t *testing.T) {
	t.Parallel()

	t.Run("Run with Control", func(t *testing.T) {
		t.Parallel()

		// Control that always returns true (should avoid execution)
		control := func(_ circuit.ExecutionPhase) bool { return true }
		observer := NewObserver(WithControl(control))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Track if the function was actually called
		executed := false
		err := observer.Run(func() error {
			executed = true
			return nil
		})

		// Should return ErrControlBreaker and not execute the function
		if !errors.Is(err, &ErrControlBreaker{}) {
			t.Errorf("Expected ErrControlBreaker, got %v", err)
		}
		if executed {
			t.Error("Function should not have been executed when Control returns true")
		}

		// Verify metrics - Control preventing execution records error and failure
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("RunFunc with Control", func(t *testing.T) {
		t.Parallel()

		// Control that always returns true (should avoid execution)
		control := func(_ circuit.ExecutionPhase) bool { return true }
		observer := NewObserver(WithControl(control))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Track if the function was actually called
		executed := false
		err := observer.RunFunc(func(_ context.Context) error {
			executed = true
			return nil
		})

		// Should return ErrControlBreaker and not execute the function
		if !errors.Is(err, &ErrControlBreaker{}) {
			t.Errorf("Expected ErrControlBreaker, got %v", err)
		}
		if executed {
			t.Error("Function should not have been executed when Control returns true")
		}

		// Verify metrics - Control preventing execution records error and failure
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("Run with Control returning false", func(t *testing.T) {
		t.Parallel()

		// Control that returns false (should allow execution)
		control := func(_ circuit.ExecutionPhase) bool { return false }
		observer := NewObserver(WithControl(control))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Track if the function was actually called
		executed := false
		err := observer.Run(func() error {
			executed = true
			return nil
		})

		// Should execute successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !executed {
			t.Error("Function should have been executed when Control returns false")
		}

		// Verify metrics - should have one success
		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("Run with nil Control", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver()
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Track if the function was actually called
		executed := false
		err := observer.Run(func() error {
			executed = true
			return nil
		})

		// Should execute successfully
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if !executed {
			t.Error("Function should have been executed when Control is nil")
		}

		// Verify metrics - should have one success
		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  0,
			Errors:    0,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})
}

func TestObserver_ControlAvoidsRetryAttempts(t *testing.T) {
	t.Parallel()

	t.Run("Control prevents retry attempts", func(t *testing.T) {
		t.Parallel()

		// Control that returns true after first attempt (should prevent retries)
		attemptCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 1 // Allow first attempt, prevent retries
		}

		observer := NewObserver(
			WithControl(control),
			WithRetrier(retry.DefaultRetrier{MaxRetries: 3}),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Function that always fails
		executionCount := 0
		err := observer.RunFunc(func(_ context.Context) error {
			executionCount++
			return errors.New("test error")
		})

		// Should fail after first attempt due to Control
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if executionCount != 1 {
			t.Errorf("Expected 1 execution, got %d", executionCount)
		}
		if attemptCount != 2 {
			t.Errorf("Expected 2 control checks (initial + retry), got %d", attemptCount)
		}

		// Verify metrics - should have 1 error, 1 failure, 0 retries
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0, // No retries due to Control
		})
	})

	t.Run("Control allows retries initially but prevents later ones", func(t *testing.T) {
		t.Parallel()

		// Control that allows first 2 attempts but prevents the 3rd retry
		attemptCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 2 // Allow first 2 attempts, prevent 3rd
		}

		observer := NewObserver(
			WithControl(control),
			WithRetrier(retry.DefaultRetrier{MaxRetries: 3}),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Function that always fails
		executionCount := 0
		err := observer.RunFunc(func(_ context.Context) error {
			executionCount++
			return errors.New("test error")
		})

		// Should fail after 2 attempts due to Control
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if executionCount != 2 {
			t.Errorf("Expected 2 executions, got %d", executionCount)
		}
		if attemptCount != 3 {
			t.Errorf("Expected 3 control checks, got %d", attemptCount)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    2, // 2 errors: one for each execution attempt
			Timeouts:  0,
			Panics:    0,
			Retries:   1, // 1 retry before Control prevented further attempts
		})
	})

	t.Run("Control with RetryBreaker interaction", func(t *testing.T) {
		t.Parallel()

		// Control that prevents execution after first attempt
		controlAttemptCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			controlAttemptCount++
			return controlAttemptCount > 1
		}

		// RetryBreaker that never triggers
		retryBreaker := func(_ error) bool {
			return false
		}

		observer := NewObserver(
			WithControl(control),
			WithRetrier(retry.DefaultRetrier{MaxRetries: 3, Breaker: retryBreaker}),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Function that always fails
		executionCount := 0
		err := observer.RunFunc(func(_ context.Context) error {
			executionCount++
			return errors.New("test error")
		})

		// Should fail after first attempt due to Control (RetryBreaker should not matter)
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if executionCount != 1 {
			t.Errorf("Expected 1 execution, got %d", executionCount)
		}

		// Verify metrics
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0, // No retries due to Control
		})
	})
}

func TestObserver_ControlWithCircuitImplementations(t *testing.T) {
	t.Parallel()

	t.Run("OnSignal Control", func(t *testing.T) {
		t.Parallel()

		// Create a signal channel
		signalCh := make(chan struct{}, 1)
		control := circuit.WhenClosed(signalCh)

		observer := NewObserver(WithControl(control))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// First execution should succeed (no signal)
		executed1 := false
		err1 := observer.Run(func() error {
			executed1 = true
			return nil
		})

		if err1 != nil {
			t.Errorf("Expected no error on first execution, got %v", err1)
		}
		if !executed1 {
			t.Error("First execution should have occurred")
		}

		// Send signal
		signalCh <- struct{}{}

		// Second execution should be avoided
		executed2 := false
		err2 := observer.Run(func() error {
			executed2 = true
			return nil
		})

		if !errors.Is(err2, &ErrControlBreaker{}) {
			t.Errorf("Expected ErrControlBreaker on second execution, got %v", err2)
		}
		if executed2 {
			t.Error("Second execution should have been avoided")
		}

		// Verify metrics - should have 1 success, 1 failure, 1 error (from second execution being prevented)
		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("OnDone Control", func(t *testing.T) {
		t.Parallel()

		// Create a done channel
		doneCh := make(chan struct{})
		control := circuit.WhenClosed(doneCh)

		observer := NewObserver(WithControl(control))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// First execution should succeed (channel not closed)
		executed1 := false
		err1 := observer.Run(func() error {
			executed1 = true
			return nil
		})

		if err1 != nil {
			t.Errorf("Expected no error on first execution, got %v", err1)
		}
		if !executed1 {
			t.Error("First execution should have occurred")
		}

		// Close done channel
		close(doneCh)

		// Second execution should be avoided
		executed2 := false
		err2 := observer.Run(func() error {
			executed2 = true
			return nil
		})

		if !errors.Is(err2, &ErrControlBreaker{}) {
			t.Errorf("Expected ErrControlBreaker on second execution, got %v", err2)
		}
		if executed2 {
			t.Error("Second execution should have been avoided")
		}

		// Verify metrics - should have 1 success, 1 failure, 1 error (from second execution being prevented)
		Verify(t, observer, metricsCounts{
			Successes: 1,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})
}

func TestObserver_ControlWithTimeout(t *testing.T) {
	t.Parallel()

	t.Run("Control with timeout configuration", func(t *testing.T) {
		t.Parallel()

		// Control that allows execution
		control := func(_ circuit.ExecutionPhase) bool { return false }
		observer := NewObserver(
			WithControl(control),
			WithTimeout(100*time.Millisecond),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Function that takes longer than timeout
		executed := false
		err := observer.RunFunc(func(ctx context.Context) error {
			executed = true
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		})

		// Should timeout
		if err == nil {
			t.Error("Expected timeout error, got nil")
		}
		if !executed {
			t.Error("Function should have been executed")
		}

		// Verify metrics - should have 1 error, 1 timeout, 1 failure
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  1,
			Panics:    0,
			Retries:   0,
		})
	})

	t.Run("Control prevents execution with timeout", func(t *testing.T) {
		t.Parallel()

		// Control that prevents execution
		control := func(_ circuit.ExecutionPhase) bool { return true }
		observer := NewObserver(
			WithControl(control),
			WithTimeout(100*time.Millisecond),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Function that would timeout
		executed := false
		err := observer.RunFunc(func(ctx context.Context) error {
			executed = true
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		})

		// Should return ErrControlBreaker, not timeout
		if !errors.Is(err, &ErrControlBreaker{}) {
			t.Errorf("Expected ErrControlBreaker, got %v", err)
		}
		if executed {
			t.Error("Function should not have been executed")
		}

		// Verify metrics - Control preventing execution records error and failure
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})
}

func TestObserver_ControlConcurrency(t *testing.T) {
	t.Parallel()

	t.Run("Control with concurrent executions", func(t *testing.T) {
		t.Parallel()

		// Control that allows first few executions then prevents further ones
		var mu sync.Mutex
		executionCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			mu.Lock()
			defer mu.Unlock()
			executionCount++
			return executionCount > 5 // Allow first 5 executions
		}

		observer := NewObserver(WithControl(control))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Run multiple goroutines concurrently
		const numGoroutines = 10
		var wg sync.WaitGroup
		results := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := observer.Run(func() error {
					time.Sleep(10 * time.Millisecond) // Small delay
					return nil
				})
				results <- err
			}()
		}

		wg.Wait()
		close(results)

		// Count results
		successCount := 0
		controlBreakerCount := 0
		for err := range results {
			if err == nil {
				successCount++
			} else if errors.Is(err, &ErrControlBreaker{}) {
				controlBreakerCount++
			} else {
				t.Errorf("Unexpected error: %v", err)
			}
		}

		// Should have some successes and some control breaker errors
		if successCount == 0 {
			t.Error("Expected some successful executions")
		}
		if controlBreakerCount == 0 {
			t.Error("Expected some control breaker errors")
		}
		if successCount+controlBreakerCount != numGoroutines {
			t.Errorf("Expected %d total results, got %d", numGoroutines, successCount+controlBreakerCount)
		}

		// Verify metrics - Control preventing execution records error and failure for each prevented execution
		Verify(t, observer, metricsCounts{
			Successes: float64(successCount),
			Failures:  float64(controlBreakerCount),
			Errors:    float64(controlBreakerCount),
			Timeouts:  0,
			Panics:    0,
			Retries:   0,
		})
	})
}

func TestObserver_ControlWithRetryStrategy(t *testing.T) {
	t.Parallel()

	t.Run("Control with retry strategy", func(t *testing.T) {
		t.Parallel()

		// Control that allows first attempt but prevents retries
		attemptCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 1
		}

		// Retry strategy that waits
		retryStrategy := retry.WaitFunc(func(_ int) time.Duration {
			return 10 * time.Millisecond
		})

		observer := NewObserver(
			WithControl(control),
			WithRetrier(retry.DefaultRetrier{MaxRetries: 2, WaitStrategy: retryStrategy}),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Function that always fails
		executionCount := 0
		start := time.Now()
		err := observer.RunFunc(func(_ context.Context) error {
			executionCount++
			return errors.New("test error")
		})
		duration := time.Since(start)

		// Should fail after first attempt due to Control
		if err == nil {
			t.Error("Expected error, got nil")
		}
		if executionCount != 1 {
			t.Errorf("Expected 1 execution, got %d", executionCount)
		}

		// Should not have waited for retry strategy since Control prevented retry
		if duration > 50*time.Millisecond {
			t.Errorf("Expected quick execution due to Control, took %v", duration)
		}

		// Verify metrics
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0,
			Retries:   0, // No retries due to Control
		})
	})
}

func TestObserver_ControlPanicRecovery(t *testing.T) {
	t.Parallel()

	t.Run("panicking Control allows execution and increments panics_total", func(t *testing.T) {
		t.Parallel()

		// Control that always panics
		observer := NewObserver(WithControl(func(_ circuit.ExecutionPhase) bool {
			panic("control panic")
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		executed := false
		err := observer.Run(func() error {
			executed = true
			return nil
		})

		// Should not crash; task should execute normally (control panic → allow)
		if err != nil {
			t.Errorf("Expected no error when Control panics, got %v", err)
		}
		if !executed {
			t.Error("Task should have executed when Control panics (allow execution)")
		}

		// panics_total should have incremented for the callback panic
		if got := testutil.ToFloat64(observer.metrics.panics); got < 1 {
			t.Errorf("Expected panics_total >= 1, got %f", got)
		}
	})

	t.Run("panicking Control during retry allows retry and increments panics_total", func(t *testing.T) {
		t.Parallel()

		// Control that panics on retry phase, allowing retries
		observer := NewObserver(
			WithControl(func(phase circuit.ExecutionPhase) bool {
				if phase == circuit.PhaseRetry {
					panic("retry control panic")
				}
				return false
			}),
			WithRetrier(retry.DefaultRetrier{MaxRetries: 2}),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		execCount := 0
		err := observer.Run(func() error {
			execCount++
			if execCount < 3 {
				return errors.New("transient error")
			}
			return nil
		})

		// Should eventually succeed after retries
		if err != nil {
			t.Errorf("Expected success after retries, got %v", err)
		}

		// panics_total should be incremented for each retry control panic
		if got := testutil.ToFloat64(observer.metrics.panics); got < 1 {
			t.Errorf("Expected panics_total >= 1 for retry control panics, got %f", got)
		}
	})
}

func TestObserver_WithControlOption(t *testing.T) {
	t.Parallel()

	t.Run("WithControl(WhenClosed) stops Run after done is closed", func(t *testing.T) {
		t.Parallel()

		done := make(chan struct{})
		observer := NewObserver(WithControl(circuit.WhenClosed(done)))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Before closing: task should run normally
		executed := false
		err := observer.Run(func() error {
			executed = true
			return nil
		})
		if err != nil {
			t.Fatalf("Expected no error before close, got: %v", err)
		}
		if !executed {
			t.Error("Expected task to execute before done is closed")
		}

		// Close the done channel
		close(done)

		// After closing: Run should return ErrControlBreaker
		executed = false
		err = observer.Run(func() error {
			executed = true
			return nil
		})
		if !errors.Is(err, &ErrControlBreaker{}) {
			t.Errorf("Expected ErrControlBreaker after close, got: %v", err)
		}
		if executed {
			t.Error("Expected task NOT to execute after done is closed")
		}
	})

	t.Run("NewObserver without WithControl runs normally", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver()
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		executed := false
		err := observer.Run(func() error {
			executed = true
			return nil
		})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if !executed {
			t.Error("Expected task to execute")
		}
	})

	t.Run("WithControl stops retries on PhaseRetry", func(t *testing.T) {
		t.Parallel()

		// Control that stops only on PhaseRetry
		observer := NewObserver(
			WithControl(func(phase circuit.ExecutionPhase) bool {
				return phase == circuit.PhaseRetry
			}),
			WithRetrier(retry.DefaultRetrier{MaxRetries: 3}),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		callCount := 0
		task := implTask{
			fn: func(_ context.Context) error {
				callCount++
				return errors.New("fail")
			},
		}
		err := observer.observe(observer.limiter, observer.control, &task)

		if err == nil {
			t.Error("Expected error from failing task")
		}
		// Should execute once (PhaseNewRequest allowed) then stop on first PhaseRetry
		if callCount != 1 {
			t.Errorf("Expected 1 execution (no retries), got %d", callCount)
		}
	})
}

func TestObserver_ErrorLabelerPanicRecovery(t *testing.T) {
	t.Parallel()

	t.Run("panicking ErrorLabeler does not crash; errors_total still increments", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(WithErrorLabels(func(_ error) prometheus.Labels {
			panic("labeler panic")
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		err := observer.Run(func() error {
			return errors.New("some error")
		})

		if err == nil {
			t.Error("Expected error from failing task")
		}

		// errors_total should still increment even though labeler panicked
		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    1, // panics_total incremented for labeler panic
			Retries:   0,
		})
	})

	t.Run("non-panicking ErrorLabeler does not increment panics_total", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(WithErrorLabels(func(_ error) prometheus.Labels {
			return prometheus.Labels{}
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		err := observer.Run(func() error {
			return errors.New("some error")
		})

		if err == nil {
			t.Error("Expected error from failing task")
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    1,
			Timeouts:  0,
			Panics:    0, // no panic in labeler
			Retries:   0,
		})
	})
}

func TestObserver_WithErrorLabels(t *testing.T) {
	t.Parallel()

	t.Run("errors_total has label dimension when WithErrorLabels is set", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(WithErrorLabels(func(err error) prometheus.Labels {
			return prometheus.Labels{"type": fmt.Sprintf("%T", err)}
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		pathErr := &os.PathError{Op: "open", Path: "/tmp/test", Err: os.ErrNotExist}
		_ = observer.Run(func() error {
			return pathErr
		})

		// errors_total should have 1 total error recorded
		got := testutil.ToFloat64(observer.metrics.errorsLabeledVec)
		if got != 1 {
			t.Errorf("Expected errors_total total=1, got %f", got)
		}

		// The label value should match the actual type name of the error
		errTypeName := fmt.Sprintf("%T", pathErr)
		got = testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": errTypeName}))
		if got != 1 {
			t.Errorf("Expected errors_total{type=%q}=1, got %f", errTypeName, got)
		}
	})

	t.Run("labeler is not called when task returns nil error", func(t *testing.T) {
		t.Parallel()

		labelerCalled := false
		observer := NewObserver(WithErrorLabels(func(err error) prometheus.Labels {
			labelerCalled = true
			return prometheus.Labels{"type": fmt.Sprintf("%T", err)}
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Reset flag after construction (discovery calls labeler once with a sample error)
		labelerCalled = false

		_ = observer.Run(func() error {
			return nil
		})

		if labelerCalled {
			t.Error("Expected labeler not to be called when task returns nil error")
		}
	})

	t.Run("without WithErrorLabels, errors_total is a plain counter", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver()
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		_ = observer.Run(func() error {
			return errors.New("some error")
		})

		// errorsLabeledVec should be nil (plain counter)
		if observer.metrics.errorsLabeledVec != nil {
			t.Error("Expected errorsLabeledVec to be nil without WithErrorLabels")
		}
		Verify(t, observer, metricsCounts{
			Errors:   1,
			Failures: 1,
		})
	})

	t.Run("different error types get different label values", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(WithErrorLabels(func(err error) prometheus.Labels {
			return prometheus.Labels{"type": fmt.Sprintf("%T", err)}
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		_ = observer.Run(func() error { return io.EOF })
		_ = observer.Run(func() error { return &os.PathError{} })
		_ = observer.Run(func() error { return io.EOF })

		// io.EOF type name
		eofTypeName := fmt.Sprintf("%T", io.EOF)
		// io.EOF should be counted twice
		eofCount := testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": eofTypeName}))
		if eofCount != 2 {
			t.Errorf("Expected errors_total{type=%q}=2, got %f", eofTypeName, eofCount)
		}

		// PathError type name (may be *fs.PathError or *os.PathError depending on Go version)
		pathTypeName := fmt.Sprintf("%T", &os.PathError{})
		pathCount := testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": pathTypeName}))
		if pathCount != 1 {
			t.Errorf("Expected errors_total{type=%q}=1, got %f", pathTypeName, pathCount)
		}
	})
}

func TestObserver_WithErrorLabels_Submit(t *testing.T) {
	t.Parallel()

	t.Run("labels errors from Submit async path", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(WithErrorLabels(func(err error) prometheus.Labels {
			return prometheus.Labels{"type": fmt.Sprintf("%T", err)}
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		observer.Submit(func() error { return io.EOF })
		if err := observer.Wait(); err == nil {
			t.Error("Expected error from failing Submit task")
		}

		eofTypeName := fmt.Sprintf("%T", io.EOF)
		got := testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": eofTypeName}))
		if got != 1 {
			t.Errorf("Expected errors_total{type=%q}=1, got %f", eofTypeName, got)
		}
	})

	t.Run("multiple Submit tasks with different error types each get labeled", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(WithErrorLabels(func(err error) prometheus.Labels {
			return prometheus.Labels{"type": fmt.Sprintf("%T", err)}
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		observer.Submit(func() error { return io.EOF })
		observer.Submit(func() error { return &os.PathError{} })
		observer.Submit(func() error { return io.EOF })
		observer.Wait() //nolint:errcheck // just drain

		eofTypeName := fmt.Sprintf("%T", io.EOF)
		eofCount := testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": eofTypeName}))
		if eofCount != 2 {
			t.Errorf("Expected errors_total{type=%q}=2, got %f", eofTypeName, eofCount)
		}

		pathTypeName := fmt.Sprintf("%T", &os.PathError{})
		pathCount := testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": pathTypeName}))
		if pathCount != 1 {
			t.Errorf("Expected errors_total{type=%q}=1, got %f", pathTypeName, pathCount)
		}
	})
}

func TestObserver_WithErrorLabels_WithRetrier(t *testing.T) {
	t.Parallel()

	t.Run("each retry attempt gets labeled independently", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(
			WithErrorLabels(func(err error) prometheus.Labels {
				return prometheus.Labels{"type": fmt.Sprintf("%T", err)}
			}),
			WithRetrier(retry.NewDefaultRetrier(retry.Immediate(), 2)),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Always fail so all 3 attempts (1 initial + 2 retries) are labeled
		_ = observer.RunFunc(func(_ context.Context) error {
			return io.EOF
		})

		eofTypeName := fmt.Sprintf("%T", io.EOF)
		got := testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": eofTypeName}))
		if got != 3 {
			t.Errorf("Expected errors_total{type=%q}=3 (1 initial + 2 retries), got %f", eofTypeName, got)
		}

		Verify(t, observer, metricsCounts{
			Successes: 0,
			Failures:  1,
			Errors:    3,
			Retries:   2,
		})
	})

	t.Run("labeler called per retry attempt in Submit async path", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(
			WithErrorLabels(func(err error) prometheus.Labels {
				return prometheus.Labels{"type": fmt.Sprintf("%T", err)}
			}),
			WithRetrier(retry.NewDefaultRetrier(retry.Immediate(), 1)),
		)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		observer.Submit(func() error { return io.EOF })
		observer.Wait() //nolint:errcheck // just drain

		eofTypeName := fmt.Sprintf("%T", io.EOF)
		got := testutil.ToFloat64(observer.metrics.errorsLabeledVec.With(prometheus.Labels{"type": eofTypeName}))
		if got != 2 {
			t.Errorf("Expected errors_total{type=%q}=2 (1 initial + 1 retry), got %f", eofTypeName, got)
		}
	})
}
