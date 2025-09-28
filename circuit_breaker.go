package sentinel

import "errors"

// CircuitBreaker determines whether a task should be retried.
// When returning true, the [Observer] will skip following retry attempts and return the error.
// Otherwise the [Observer] will continue to retry the task.
type CircuitBreaker func(err error) bool

// DeafultCircuitBreaker always allows retries regardless of the error.
var DeafultCircuitBreaker CircuitBreaker = nil

// CircuitBreakerShortOnPanic is a CircuitBreaker that exits on panic occurrences.
var CircuitBreakerShortOnPanic = func(err error) bool {
	return errors.Is(err, ErrPanicOccurred)
}
