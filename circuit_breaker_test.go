package sentinel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// testTask is a test implementation of the Task interface for circuit breaker testing.
type circuitBreakerTestTask struct {
	cfg            TaskConfig
	attemptCount   int
	shouldPanic    bool
	shouldSucceed  bool
	panicOnAttempt int // panic on specific attempt (0 = first attempt, 1 = first retry, etc.)
}

var _ Task = (*circuitBreakerTestTask)(nil)

func (t *circuitBreakerTestTask) Config() TaskConfig {
	return t.cfg
}

func (t *circuitBreakerTestTask) Execute(ctx context.Context) error {
	t.attemptCount++

	// Check if we should panic on this specific attempt
	if t.shouldPanic && t.attemptCount == t.panicOnAttempt {
		panic("test panic on attempt " + string(rune(t.attemptCount)))
	}

	// If we should succeed after a certain number of attempts
	if t.shouldSucceed && t.attemptCount >= 3 {
		return nil
	}

	// Return a regular error for retry testing
	return errors.New("task failed on attempt " + string(rune(t.attemptCount)))
}

func TestCircuitBreaker_DefaultCircuitBreaker(t *testing.T) {
	t.Run("allows retries on regular errors", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: DefaultCircuitBreaker, // nil - allows all retries
			},
			shouldSucceed: true, // succeed after 3 attempts
		}

		err := observer.RunTask(task)

		// Should succeed after retries
		if err != nil {
			t.Errorf("Expected no error after successful retries, got %v", err)
		}

		// Should have made 3 attempts (1 initial + 2 retries)
		if task.attemptCount != 3 {
			t.Errorf("Expected 3 attempts, got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 2 {
			t.Errorf("Expected Errors=2 (first 2 attempts failed), got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 2 {
			t.Errorf("Expected Retries=2, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 1 {
			t.Errorf("Expected Successes=1, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Panics); got != 0 {
			t.Errorf("Expected Panics=0, got %f", got)
		}
	})

	t.Run("allows retries on panic errors", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          2,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: DefaultCircuitBreaker, // nil - allows all retries
			},
			shouldPanic:    true,
			panicOnAttempt: 1, // panic on first attempt
		}

		err := observer.RunTask(task)

		// Should return panic error after all retries exhausted
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		var panicErr ErrPanicOccurred
		if !errors.As(err, &panicErr) {
			t.Errorf("Expected ErrPanicOccurred, got %v", err)
		}

		// Should have made 3 attempts (1 initial + 2 retries)
		if task.attemptCount != 3 {
			t.Errorf("Expected 3 attempts, got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 3 {
			t.Errorf("Expected Errors=3, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 2 {
			t.Errorf("Expected Retries=2, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Panics); got != 3 {
			t.Errorf("Expected Panics=3 (all attempts panicked), got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
			t.Errorf("Expected Successes=0, got %f", got)
		}
	})

	t.Run("continues retrying until max retries reached", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          5,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: DefaultCircuitBreaker, // nil - allows all retries
			},
			shouldSucceed: false, // never succeed
		}

		err := observer.RunTask(task)

		// Should return error after all retries exhausted
		if err == nil {
			t.Errorf("Expected error after exhausted retries, got nil")
		}

		// Should have made 6 attempts (1 initial + 5 retries)
		if task.attemptCount != 6 {
			t.Errorf("Expected 6 attempts, got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 6 {
			t.Errorf("Expected Errors=6, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 5 {
			t.Errorf("Expected Retries=5, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
			t.Errorf("Expected Successes=0, got %f", got)
		}
	})
}

func TestCircuitBreaker_ShortOnPanicCircuitBreaker(t *testing.T) {
	t.Run("allows retries on regular errors", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: ShortOnPanicCircuitBreaker,
			},
			shouldSucceed: true, // succeed after 3 attempts
		}

		err := observer.RunTask(task)

		// Should succeed after retries (regular errors don't trigger circuit breaker)
		if err != nil {
			t.Errorf("Expected no error after successful retries, got %v", err)
		}

		// Should have made 3 attempts (1 initial + 2 retries)
		if task.attemptCount != 3 {
			t.Errorf("Expected 3 attempts, got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 2 {
			t.Errorf("Expected Errors=2 (first 2 attempts failed), got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 2 {
			t.Errorf("Expected Retries=2, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 1 {
			t.Errorf("Expected Successes=1, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Panics); got != 0 {
			t.Errorf("Expected Panics=0, got %f", got)
		}
	})

	t.Run("stops retries immediately on panic", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          5, // Would normally retry 5 times
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: ShortOnPanicCircuitBreaker,
			},
			shouldPanic:    true,
			panicOnAttempt: 1, // panic on first attempt
		}

		err := observer.RunTask(task)

		// Should return panic error immediately (no retries)
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		var panicErr ErrPanicOccurred
		if !errors.As(err, &panicErr) {
			t.Errorf("Expected ErrPanicOccurred, got %v", err)
		}

		// Should have made only 1 attempt (circuit breaker stopped retries)
		if task.attemptCount != 1 {
			t.Errorf("Expected 1 attempt (circuit breaker stopped retries), got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 1 {
			t.Errorf("Expected Errors=1, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 0 {
			t.Errorf("Expected Retries=0 (circuit breaker prevented retries), got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Panics); got != 1 {
			t.Errorf("Expected Panics=1, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
			t.Errorf("Expected Successes=0, got %f", got)
		}
	})

	t.Run("stops retries on panic during retry attempt", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          5,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: ShortOnPanicCircuitBreaker,
			},
			shouldPanic:    true,
			panicOnAttempt: 2, // panic on first retry attempt
		}

		err := observer.RunTask(task)

		// Should return panic error after first retry attempt
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		var panicErr ErrPanicOccurred
		if !errors.As(err, &panicErr) {
			t.Errorf("Expected ErrPanicOccurred, got %v", err)
		}

		// Should have made only 2 attempts (1 initial + 1 retry that panicked)
		if task.attemptCount != 2 {
			t.Errorf("Expected 2 attempts (circuit breaker stopped after panic on retry), got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 2 {
			t.Errorf("Expected Errors=2, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 1 {
			t.Errorf("Expected Retries=1 (only first retry attempted), got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Panics); got != 1 {
			t.Errorf("Expected Panics=1 (only second attempt panicked), got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
			t.Errorf("Expected Successes=0, got %f", got)
		}
	})

	t.Run("continues retrying regular errors until max retries", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: ShortOnPanicCircuitBreaker,
			},
			shouldSucceed: false, // never succeed, but no panics
		}

		err := observer.RunTask(task)

		// Should return error after all retries exhausted
		if err == nil {
			t.Errorf("Expected error after exhausted retries, got nil")
		}

		// Should have made 4 attempts (1 initial + 3 retries)
		if task.attemptCount != 4 {
			t.Errorf("Expected 4 attempts, got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Errors); got != 4 {
			t.Errorf("Expected Errors=4, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 3 {
			t.Errorf("Expected Retries=3, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Panics); got != 0 {
			t.Errorf("Expected Panics=0, got %f", got)
		}
		if got := testutil.ToFloat64(observer.metrics.Successes); got != 0 {
			t.Errorf("Expected Successes=0, got %f", got)
		}
	})
}

func TestCircuitBreaker_Comparison(t *testing.T) {
	t.Run("compare behavior on panic scenarios", func(t *testing.T) {
		// Test with DefaultCircuitBreaker
		observer1 := NewObserver(testConfig(t))
		registry1 := prometheus.NewRegistry()
		observer1.MustRegister(registry1)

		task1 := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: DefaultCircuitBreaker,
			},
			shouldPanic:    true,
			panicOnAttempt: 1, // panic on first attempt
		}

		err1 := observer1.RunTask(task1)

		// Test with ShortOnPanicCircuitBreaker
		observer2 := NewObserver(testConfig(t))
		registry2 := prometheus.NewRegistry()
		observer2.MustRegister(registry2)

		task2 := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: ShortOnPanicCircuitBreaker,
			},
			shouldPanic:    true,
			panicOnAttempt: 1, // panic on first attempt
		}

		err2 := observer2.RunTask(task2)

		// Both should return panic errors
		if err1 == nil || err2 == nil {
			t.Errorf("Both should return panic errors, got err1=%v, err2=%v", err1, err2)
		}

		// DefaultCircuitBreaker should have made more attempts
		if task1.attemptCount <= task2.attemptCount {
			t.Errorf("DefaultCircuitBreaker should make more attempts than ShortOnPanicCircuitBreaker, got %d vs %d",
				task1.attemptCount, task2.attemptCount)
		}

		// DefaultCircuitBreaker should have more retries
		retries1 := testutil.ToFloat64(observer1.metrics.Retries)
		retries2 := testutil.ToFloat64(observer2.metrics.Retries)
		if retries1 <= retries2 {
			t.Errorf("DefaultCircuitBreaker should have more retries than ShortOnPanicCircuitBreaker, got %f vs %f",
				retries1, retries2)
		}

		// Both should have recorded panics
		panics1 := testutil.ToFloat64(observer1.metrics.Panics)
		panics2 := testutil.ToFloat64(observer2.metrics.Panics)
		if panics1 == 0 || panics2 == 0 {
			t.Errorf("Both should have recorded panics, got panics1=%f, panics2=%f", panics1, panics2)
		}
	})

	t.Run("compare behavior on regular error scenarios", func(t *testing.T) {
		// Test with DefaultCircuitBreaker
		observer1 := NewObserver(testConfig(t))
		registry1 := prometheus.NewRegistry()
		observer1.MustRegister(registry1)

		task1 := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: DefaultCircuitBreaker,
			},
			shouldSucceed: false, // never succeed, no panics
		}

		err1 := observer1.RunTask(task1)

		// Test with ShortOnPanicCircuitBreaker
		observer2 := NewObserver(testConfig(t))
		registry2 := prometheus.NewRegistry()
		observer2.MustRegister(registry2)

		task2 := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: ShortOnPanicCircuitBreaker,
			},
			shouldSucceed: false, // never succeed, no panics
		}

		err2 := observer2.RunTask(task2)

		// Both should return errors after exhausting retries
		if err1 == nil || err2 == nil {
			t.Errorf("Both should return errors after exhausting retries, got err1=%v, err2=%v", err1, err2)
		}

		// Both should have made the same number of attempts
		if task1.attemptCount != task2.attemptCount {
			t.Errorf("Both should make same number of attempts for regular errors, got %d vs %d",
				task1.attemptCount, task2.attemptCount)
		}

		// Both should have the same number of retries
		retries1 := testutil.ToFloat64(observer1.metrics.Retries)
		retries2 := testutil.ToFloat64(observer2.metrics.Retries)
		if retries1 != retries2 {
			t.Errorf("Both should have same number of retries for regular errors, got %f vs %f",
				retries1, retries2)
		}

		// Neither should have recorded panics
		panics1 := testutil.ToFloat64(observer1.metrics.Panics)
		panics2 := testutil.ToFloat64(observer2.metrics.Panics)
		if panics1 != 0 || panics2 != 0 {
			t.Errorf("Neither should have recorded panics for regular errors, got panics1=%f, panics2=%f",
				panics1, panics2)
		}
	})
}

func TestCircuitBreaker_EdgeCases(t *testing.T) {
	t.Run("nil circuit breaker behaves like DefaultCircuitBreaker", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          2,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: nil, // nil should behave like DeafultCircuitBreaker
			},
			shouldPanic:    true,
			panicOnAttempt: 1, // panic on first attempt
		}

		err := observer.RunTask(task)

		// Should return panic error after all retries
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		// Should have made 3 attempts (1 initial + 2 retries)
		if task.attemptCount != 3 {
			t.Errorf("Expected 3 attempts with nil circuit breaker, got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 2 {
			t.Errorf("Expected Retries=2 with nil circuit breaker, got %f", got)
		}
	})

	t.Run("circuit breaker with custom error type", func(t *testing.T) {
		observer := NewObserver(testConfig(t))
		registry := prometheus.NewRegistry()
		observer.MustRegister(registry)

		// Custom circuit breaker that breaks on specific error type
		customCircuitBreaker := func(err error) bool {
			return errors.Is(err, &ErrPanicOccurred{})
		}

		task := &circuitBreakerTestTask{
			cfg: TaskConfig{
				Timeout:             time.Second,
				MaxRetries:          3,
				Concurrent:          false,
				RecoverPanics:       true,
				RetryStrategy:       RetryStrategyImmediate,
				RetryCircuitBreaker: customCircuitBreaker,
			},
			shouldPanic:    true,
			panicOnAttempt: 1, // panic on first attempt
		}

		err := observer.RunTask(task)

		// Should return panic error immediately (circuit breaker stops retries)
		if err == nil {
			t.Errorf("Expected panic error, got nil")
		}

		// Should have made only 1 attempt
		if task.attemptCount != 1 {
			t.Errorf("Expected 1 attempt with custom circuit breaker, got %d", task.attemptCount)
		}

		// Verify metrics
		if got := testutil.ToFloat64(observer.metrics.Retries); got != 0 {
			t.Errorf("Expected Retries=0 with custom circuit breaker, got %f", got)
		}
	})
}
