package sentinel

import (
	"errors"
	"time"
)

// NewPanicCircuitBreaker returns a handler with the CircuitBreaker signature
// that returns true when a [ErrPanicOccurred] error is encountered. This is
// useful to stop retries after a panic occurs.
func NewPanicCircuitBreaker() func(err error) bool {
	return func(err error) bool {
		var panicErr *ErrPanicOccurred
		return errors.As(err, &panicErr)
	}
}

// NewTimeoutCircuitBreaker returns a handler with the CircuitBreaker signature
// that returns true when processing has taken longer than the timeout duration.
func NewTimeoutCircuitBreaker(timeout time.Duration) func(err error) bool {
	var start = time.Now()
	return func(err error) bool {
		return time.Since(start) > timeout
	}
}
