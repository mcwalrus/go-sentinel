package sentinel

import (
	"context"
	"time"

	"github.com/mcwalrus/go-sentinel/circuit"
	"github.com/mcwalrus/go-sentinel/retry"
)

type implTask struct {
	fn         func(ctx context.Context) error
	cfg        TaskConfig
	retryCount int
}

type TaskConfig struct {
	// Timeout is a context deadline for [Task.Execute]. By default, no timeout is applied.
	// It is the responsibility of the [Task] to handle the deadline error whenever exceeded.
	// The [Observer] records the timeout occurrences via metrics. The timeout will not be
	// respected if the observer Run* func does not pass a context.
	Timeout time.Duration

	// MaxRetries specifies the number of retry attempts for failed calls of [Task.Execute].
	// If set to zero, no retries are performed. Each retry attempt is recorded via metrics.
	// Errors returned from multiple retries by the [Observer] will be grouped by [errors.Join]
	// as a single error.
	MaxRetries int

	// RetryStrategy is a handler to return wait durations between retry attempts. The first
	// wait duration requested by the handler will provide retryCount at 0. Subsequent retries
	// will increment retryCount. By default, no wait strategy is applied by the [Observer].
	// Use the retry package for common strategies like retry.Exponential, retry.WithJitter,
	// etc.
	RetryStrategy retry.Strategy

	// RetryBreaker is a handler that when will avoid all following retry attempts when
	// true is returned. The handler will be provided the error from the previous attempt.
	// When nil, the [Observer] will always attempt the next retry. This is useful to stop
	// retries when certain errors or conditions have occurred.
	RetryBreaker circuit.Breaker

	// Control is a handler that when will avoid all following retry attempts when returning
	// true. The handler will be provided the error from the previous attempt.
	// When nil, the [Observer] will always attempt the next retry. This is useful to stop
	// retries when certain errors or conditions have occurred.
	Control circuit.Control
}
