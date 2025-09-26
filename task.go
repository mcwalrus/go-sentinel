package sentinel

import (
	"context"
	"time"
)

// Task represents a unit of work that can be executed and monitored by an [Observer].
// Implementations can describe workloads alternatively than using a functional approach.
type Task interface {
	// Config returns the [TaskConfig] for how the task will be handled by the [Observer].
	// This provides task retry behavior, concurrency settings, and panic recovery options.
	Config() TaskConfig

	// Execute performs the task work. If a [context.DeadlineExceeded] error is returned,
	// it is recorded as a timeout occurrence by the [Observer]. The [TaskConfig] timeout
	// is used by the context passed to the method.
	Execute(ctx context.Context) error
}

// TaskConfig defines the configuration options for task execution and monitoring.
// The [Observer] uses this configuration to determine the execution behavior of a [Task].

type TaskConfig struct {
	// Timeout duration is the deadline for the context passed to [Task.Execute].
	// A zero value means no timeout is applied. When the timeout is exceeded, the context
	// will return [context.DeadlineExceeded], and it is the responsibility of the [Task]
	// to handle and return the error.
	Timeout time.Duration

	// MaxRetries specifies the maximum number of retry attempts for failed [Task.Execute].
	// If set to zero, no retries are performed. Each retry attempt is recorded via metrics.
	// The [Observer] records the metrics for each attempt to run the [Task] individually.
	MaxRetries int

	// Concurrent determines whether the task should run asynchronously.
	// If true, the task runs in a goroutine and the [Observer] methods will return immediately.
	// If false, the task runs synchronously and the [Observer] methods will block until completed
	// returning any associated errors. On multiple synchronous failures, the errors will be
	// returned by [errors.Join].
	Concurrent bool

	// RecoverPanics determines whether panics should be caught and recover gracefully,
	// or ignored and propagate up the stack. When true, panics are caught, recorded via metrics,
	// where an error is returned by the [Observer] indicating that a panic occurred. When false,
	// panics are still recorded, but can propagate and potentially crash the program.
	RecoverPanics bool

	// RetryStrategy defines the wait duration calculation between retry attempts.
	// The function receives the number of retries already attempted and should return the
	// duration to wait before the next retry (0 for first retry).
	// If nil, [RetryStrategyImmediate] is used.
	RetryStrategy func(retryCount int) time.Duration

	// RetryCircuitBreaker determines whether the task will avoid the next retry attempt.
	// If returns true, the [Observer] will break the retry attempt and return the error.
	// If returns false, the [Observer] will continue to retry the task.
	RetryCircuitBreaker func(err error) bool
}

func defaultTaskConfig() TaskConfig {
	return TaskConfig{
		Timeout:             0,
		MaxRetries:          0,
		Concurrent:          false,
		RecoverPanics:       true,
		RetryStrategy:       RetryStrategyImmediate,
		RetryCircuitBreaker: nil,
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
