package sentinel

import (
	_ "context"
	"time"

	"github.com/mcwalrus/go-sentinel/circuit"
	"github.com/mcwalrus/go-sentinel/retry"
)

type ObserverConfig struct {
	// Timeout sets a context deadline tasks passed to [Observer.RunFunc].
	// The Observer records timeout occurrences via metrics when enabled.
	Timeout time.Duration

	// MaxRetries specifies the number of retry attempts for tasks on errors. If a task
	// fails for all attempts, the observer groups errors from multiple attempts using
	// [errors.Join]. By default, no retries are performed.
	MaxRetries int

	// RetryStrategy is a handler which returns wait durations between retry attempts.
	// The first call to the handler will provide retryCount at 0. Subsequent calls will
	// increment retryCount. By default, no wait strategy is applied.
	RetryStrategy retry.WaitFunc

	// RetryBreaker is a handler that skips following retry attempts for a task when
	// returning true. The handler will be provided the error from the previous attempt.
	// When nil, the observer will always attempt the next retry. This is useful to stop
	// retries on particular errors.
	RetryBreaker circuit.Breaker

	// Control receives the execution phase (PhaseNewRequest for new task executions, or
	// PhaseRetry for retry attempts) and returns true to cancel execution. This is useful
	// to manage or avoid new task executions on shutdown signals, or handling of specific
	// errors. When nil, all requests and retries are allowed.
	Control circuit.Control
}
