package sentinel

import (
	_ "context" // for documentation references to context.Context
	"time"

	"github.com/mcwalrus/go-sentinel/v2/circuit"
	"github.com/mcwalrus/go-sentinel/v2/retry"
)

// ObserverConfig configures observer behavior for timeouts, retries, concurrency, and control.
//
// Deprecated: Use construction-time ObserverOption functions instead:
//   - [WithTimeout] for Timeout
//   - [WithRetrier] for MaxRetries/RetryStrategy/RetryBreaker
//   - [WithControl] for Control
//   - [WithMaxConcurrency] for MaxConcurrency
type ObserverConfig struct {
	// Timeout sets a context deadline for functions passed to [Observer.RunFunc].
	// The observer records timeout occurrences under the "timeouts" metric when enabled.
	// The timeout is applied per iterative attempt to run the function.
	Timeout time.Duration

	// MaxRetries specifies the number of retry attempts for tasks on errors. If a task
	// fails for all attempts, the observer groups errors from multiple attempts using
	// [errors.Join]. By default, no retries are performed.
	//
	// Deprecated: Use [sentinel.WithRetrier] with a [retry.DefaultRetrier] instead.
	// When an observer-level Retrier is configured via WithRetrier(), MaxRetries is ignored.
	MaxRetries int

	// RetryStrategy is a handler which returns wait durations between retry attempts.
	// The first retry attempt will call the handler with retryCount=1. By default,
	// no wait strategy is applied (immediate retry).
	//
	// Deprecated: Use [sentinel.WithRetrier] with a [retry.DefaultRetrier] instead.
	// When an observer-level Retrier is configured via WithRetrier(), RetryStrategy is ignored.
	RetryStrategy retry.WaitFunc

	// RetryBreaker is a handler that skips following retry attempts for a task when
	// returning true. The handler will be provided the error from the previous attempt.
	// When nil, the observer will always attempt the next retry. This is useful to stop
	// retries on particular errors.
	//
	// Deprecated: Use [sentinel.WithRetrier] with a [retry.DefaultRetrier] instead.
	// When an observer-level Retrier is configured via WithRetrier(), RetryBreaker is ignored.
	RetryBreaker func(err error) bool

	// Control receives the execution phase (PhaseNewRequest for new task executions, or
	// PhaseRetry for retry attempts) and returns true to cancel execution. This is useful
	// to manage or avoid new task executions on shutdown signals, or handling of specific
	// errors. When nil, all requests and retries are allowed.
	Control circuit.Control

	// MaxConcurrency limits the number of concurrent task executions. When set to a value
	// greater than 0, the observer will use a semaphore to limit concurrent executions.
	// This is useful for rate limiting or preventing resource exhaustion from too many
	// concurrent operations.
	MaxConcurrency int
}
