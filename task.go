package sentinel

import (
	"context"
	"time"
)

// Task represents work that can be executed and observed by an [Observer].
// Implementations can describe workloads alternative than using a functional approach.
type Task interface {
	// Config returns the [TaskConfig] for how to handle the task through the [Observer].
	// This provides task retry behaviour, timeout duration, and panic recovery options.
	Config() TaskConfig

	// Execute performs work of the task. The [Observer] will record the execution metrics
	// for duration, success/failure, panic, and retries. The context should be respected
	// by the method, where any [context.DeadlineExceeded] errors will be recocrded as
	// timeout occurrences. Panic occurrences are always to be recorded by the [Observer].
	Execute(ctx context.Context) error
}

// TaskConfig defines the configuration options for task execution and monitoring.
// The [Observer] uses this configuration to determine the execution behaviour of a [Task].
type TaskConfig struct {
	// Timeout is a context deadline for [Task.Execute]. By default, no timeout is applied.
	// It is the responsibility of the [Task] to handle the deadline error whenever exceeded.
	// The [Observer] records the timeout occurrences via metrics.
	Timeout time.Duration

	// MaxRetries specifies the number of retry attempts for failed calls of [Task.Execute].
	// If set to zero, no retries are performed. Each retry attempt is recorded via metrics.
	// Errors returned from multiple retries are grouped by [errors.Join] as a single error.
	MaxRetries int

	// RetryStrategy is a handler to return wait durations between retry attempts. The first
	// wait duration requested by the handler will provide retryCount at 0. Subsequent retries
	// will increment retryCount. By default, no wait strategy is applied by the [Observer].
	// Use the retry package for common strategies like retry.Exponential, retry.Linear, etc.
	RetryStrategy func(retryCount int) time.Duration

	// CircuitBreaker is a handler that when will avoid all following retry attempts when
	// true is returned. The handler will be provided the error from the previous attempt.
	// When nil, the [Observer] will always attempt the next retry. This is useful to stop
	// retries when certain errors or conditions have occurred.
	CircuitBreaker func(err error) bool
}

type implTask struct {
	fn         func(ctx context.Context) error
	cfg        TaskConfig
	retryCount int
}

func (t *implTask) Config() TaskConfig {
	return t.cfg
}

func (t *implTask) Execute(ctx context.Context) error {
	return t.fn(ctx)
}
