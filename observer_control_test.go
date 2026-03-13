package sentinel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mcwalrus/go-sentinel/circuit"
)

func TestObserver_ControlAvoidsInitialExecution(t *testing.T) {
	t.Parallel()

	t.Run("Run with Control", func(t *testing.T) {
		t.Parallel()

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that always returns true (should avoid execution)
		control := func(_ circuit.ExecutionPhase) bool { return true }

		observer.UseConfig(ObserverConfig{
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that always returns true (should avoid execution)
		control := func(_ circuit.ExecutionPhase) bool { return true }

		observer.UseConfig(ObserverConfig{
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that returns false (should allow execution)
		control := func(_ circuit.ExecutionPhase) bool { return false }

		observer.UseConfig(ObserverConfig{
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// No Control set (should allow execution)
		observer.UseConfig(ObserverConfig{})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that returns true after first attempt (should prevent retries)
		attemptCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 1 // Allow first attempt, prevent retries
		}

		observer.UseConfig(ObserverConfig{
			MaxRetries: 3,
			Control:    control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that allows first 2 attempts but prevents the 3rd retry
		attemptCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 2 // Allow first 2 attempts, prevent 3rd
		}

		observer.UseConfig(ObserverConfig{
			MaxRetries: 3,
			Control:    control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

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

		observer.UseConfig(ObserverConfig{
			MaxRetries:   3,
			RetryBreaker: retryBreaker,
			Control:      control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Create a signal channel
		signalCh := make(chan struct{}, 1)
		control := circuit.WhenClosed(signalCh)

		observer.UseConfig(ObserverConfig{
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Create a done channel
		doneCh := make(chan struct{})
		control := circuit.WhenClosed(doneCh)

		observer.UseConfig(ObserverConfig{
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that allows execution
		control := func(_ circuit.ExecutionPhase) bool { return false }

		observer.UseConfig(ObserverConfig{
			Timeout: 100 * time.Millisecond,
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that prevents execution
		control := func(_ circuit.ExecutionPhase) bool { return true }

		observer.UseConfig(ObserverConfig{
			Timeout: 100 * time.Millisecond,
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that allows first few executions then prevents further ones
		var mu sync.Mutex
		executionCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			mu.Lock()
			defer mu.Unlock()
			executionCount++
			return executionCount > 5 // Allow first 5 executions
		}

		observer.UseConfig(ObserverConfig{
			Control: control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that allows first attempt but prevents retries
		attemptCount := 0
		control := func(_ circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 1
		}

		// Retry strategy that waits
		retryStrategy := func(_ int) time.Duration {
			return 10 * time.Millisecond
		}

		observer.UseConfig(ObserverConfig{
			MaxRetries:    2,
			RetryStrategy: retryStrategy,
			Control:       control,
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that always panics
		observer.UseConfig(ObserverConfig{
			Control: func(_ circuit.ExecutionPhase) bool {
				panic("control panic")
			},
		})

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

		observer := NewObserver(nil)
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Control that panics on retry phase, allowing retries
		observer.UseConfig(ObserverConfig{
			MaxRetries: 2,
			Control: func(phase circuit.ExecutionPhase) bool {
				if phase == circuit.PhaseRetry {
					panic("retry control panic")
				}
				return false
			},
		})

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
		observer := NewObserver(nil, WithControl(circuit.WhenClosed(done)))
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

		observer := NewObserver(nil)
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
		observer := NewObserver(nil, WithControl(func(phase circuit.ExecutionPhase) bool {
			return phase == circuit.PhaseRetry
		}))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		callCount := 0
		task := implTask{
			cfg: ObserverConfig{MaxRetries: 3},
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

		observer := NewObserver(nil, WithErrorLabels(func(_ error) prometheus.Labels {
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

		observer := NewObserver(nil, WithErrorLabels(func(_ error) prometheus.Labels {
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
