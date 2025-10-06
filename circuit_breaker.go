package sentinel

import "errors"

// ShortOnPanicCircuitBreaker is a handler that returns true when the error
// is an [ErrPanicOccurred] indicating a panic occurred. This is useful to
// stop retries after a panic occurs.
var ShortOnPanicCircuitBreaker = func(err error) bool {
	var panicErr *ErrPanicOccurred
	return errors.As(err, &panicErr)
}
