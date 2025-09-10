package sentinel

import (
	"context"
	"time"
)

// Task represents a unit of work that can be executed and monitored by an [Observer].
// Implementations can alternatively describe workloads instead of a functional approach.
type Task interface {
	// Config returns the configuration for a task to be handled by the [Observer],
	// including timeout, retry behavior, concurrency settings, and panic recovery options.
	Config() TaskConfig

	// Execute performs the task work. The provided context may include timeout constraints
	// based on the task configuration. If a [context.DeadlineExceeded] error is returned,
	// it is recorded as a timeout occurrence by the [Observer].
	Execute(ctx context.Context) error
}

// TaskConfig defines the configuration options for task execution and monitoring.
// The [Observer] uses this configuration to determine the execution behavior of a task.
type TaskConfig struct {
	// Timeout specifies a context timeout duration for task execution.
	// If set to 0, no timeout is applied. The context passed to Execute()
	// will be cancelled when the timeout is reached.
	Timeout time.Duration

	// MaxRetries specifies the maximum number of retry attempts for failed tasks.
	// If set to 0, no retries are performed. Each retry attempt is recorded via metrics.
	// The [Observer] records the metrics for each attempt to run the task individually.
	MaxRetries int

	// Concurrent determines whether the task should run asynchronously.
	// If true, the task runs in a goroutine and the [Observer] methods will return immediately.
	// If false, the task runs synchronously and the [Observer] methods will block until completed
	// returning any associated errors. On multiple synchronous failures, the errors will be
	// returned by [errors.Join].
	Concurrent bool

	// RecoverPanics determines whether panics should be caught and recovered.
	// If true, panics are caught, recorded via metrics, and the task execution continues.
	// If false, panics are still recorded, but can propagate and potentially crash the program.
	RecoverPanics bool

	// RetryStrategy defines the wait duration calculation between retry attempts.
	// The function receives the number of retries already attempted and should return the
	// duration to wait before the next retry (0 for first retry).
	// If nil, [RetryStrategyImmediate] is used.
	RetryStrategy func(retryCount int) time.Duration

	// RetryCurcuitBreaker determines whether the task will avoid the next retry attempt.
	// If returns true, the [Observer] will break the retry attempt and return the error.
	// If returns false, the [Observer] will continue to retry the task.
	RetryCurcuitBreaker func(err error) bool
}

func defaultTaskConfig() TaskConfig {
	return TaskConfig{
		Timeout:       0,
		MaxRetries:    0,
		Concurrent:    false,
		RecoverPanics: false,
		RetryStrategy: RetryStrategyImmediate,
		RetryCurcuitBreaker: func(err error) bool {
			return false
		},
	}
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
