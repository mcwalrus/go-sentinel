package sentinel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

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
		control := func(phase circuit.ExecutionPhase) bool { return true }

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
		control := func(phase circuit.ExecutionPhase) bool { return true }

		observer.UseConfig(ObserverConfig{
			Control: control,
		})

		// Track if the function was actually called
		executed := false
		err := observer.RunFunc(func(ctx context.Context) error {
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
		control := func(phase circuit.ExecutionPhase) bool { return false }

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
		control := func(phase circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 1 // Allow first attempt, prevent retries
		}

		observer.UseConfig(ObserverConfig{
			MaxRetries: 3,
			Control:    control,
		})

		// Function that always fails
		executionCount := 0
		err := observer.RunFunc(func(ctx context.Context) error {
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
		control := func(phase circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 2 // Allow first 2 attempts, prevent 3rd
		}

		observer.UseConfig(ObserverConfig{
			MaxRetries: 3,
			Control:    control,
		})

		// Function that always fails
		executionCount := 0
		err := observer.RunFunc(func(ctx context.Context) error {
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
		control := func(phase circuit.ExecutionPhase) bool {
			controlAttemptCount++
			return controlAttemptCount > 1
		}

		// RetryBreaker that never triggers
		retryBreaker := func(err error) bool {
			return false
		}

		observer.UseConfig(ObserverConfig{
			MaxRetries:   3,
			RetryBreaker: retryBreaker,
			Control:      control,
		})

		// Function that always fails
		executionCount := 0
		err := observer.RunFunc(func(ctx context.Context) error {
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
		control := func(phase circuit.ExecutionPhase) bool { return false }

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
		control := func(phase circuit.ExecutionPhase) bool { return true }

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
		control := func(phase circuit.ExecutionPhase) bool {
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
		control := func(phase circuit.ExecutionPhase) bool {
			attemptCount++
			return attemptCount > 1
		}

		// Retry strategy that waits
		retryStrategy := func(retryCount int) time.Duration {
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
		err := observer.RunFunc(func(ctx context.Context) error {
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
