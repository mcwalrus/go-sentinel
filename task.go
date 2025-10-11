package sentinel

import (
	"context"
	_ "errors"
	"time"

	"github.com/mcwalrus/go-sentinel/circuit"
	"github.com/mcwalrus/go-sentinel/retry"
)

type implTask struct {
	fn         func(ctx context.Context) error
	cfg        Config
	retryCount int
}

type Config struct {
	// Timeout sets a context deadline tasks passed to [Observer.RunFunc].
	// The Observer records timeout occurrences via metrics when enabled.
	Timeout time.Duration

	// MaxRetries specifies the number of retry attempts for tasks on errors.
	// If a task fails for all attempts, the observer groups errors from multiple
	// attempts using [errors.Join]. By default, no retries are performed.
	MaxRetries int

	// RetryStrategy is a handler which returns wait durations between retry attempts.
	// The first call to the handler will provide retryCount at 0. Subsequent calls
	// will increment retryCount. By default, no wait strategy is applied.
	RetryStrategy retry.Strategy

	// RetryBreaker is a handler that skips following retry attempts for a task when
	// returning true. The handler will be provided the error from the previous attempt.
	// When nil, the observer will always attempt the next retry. This is useful to stop
	// retries on particular errors.
	RetryBreaker circuit.Breaker

	// Control is a handler that avoids running any tasks or retry attempt when returning
	// true. This is useful to manage control of task execution through the observer on
	// system events such as shutdown signals, resource management, or other. When nil,
	// normal processing continues.
	Control circuit.Control
}
