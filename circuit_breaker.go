package sentinel

import "errors"

var (
	_ CircuitBreaker = DefaultCircuitBreaker
	_ CircuitBreaker = ShortOnPanicCircuitBreaker
)

// CircuitBreaker determines whether to stop retrying after an error occurs.
// When returning true, retries are stopped and the error is returned immediately.
// When returning false, the Observer continues with retry attempts according to its policy.
// You can provide your own implementation to handle specific error types or conditions.
type CircuitBreaker func(err error) bool

// DefaultCircuitBreaker always allows retries regardless of the error.
var DefaultCircuitBreaker CircuitBreaker = nil

// ShortOnPanicCircuitBreaker is a CircuitBreaker that exits on panic occurrences.
var ShortOnPanicCircuitBreaker = func(err error) bool {
	return errors.Is(err, &ErrPanicOccurred{})
}
