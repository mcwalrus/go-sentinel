// Package circuit provides circuit breaker predicates for sentinel's retry logic.
package circuit

import (
	"errors"
	"time"
)

// Breaker determines whether attempt to run task should stop given the latest error.
// It returns true to stop further attempts to run tasks. False will allow attempts to
// continue. The breaker will always expect an error to be provided. Provide your own
// breaker by implementing the function signature.
//
// Example usage:
//
//	breaker := circuit.Breaker(func(err error) bool {
//		return errors.Is(err, context.Canceled)
//	})
type Breaker func(err error) bool

// After stops retries when total elapsed time since breaker creation exceeds d.
// This is useful for implementing time-based circuit breaking, ensuring retries
// don't continue past a certain duration.
//
// Example usage:
//
//	breaker := circuit.After(30 * time.Second)
//	observer.UseConfig(sentinel.ObserverConfig{
//		MaxRetries:  5,
//		RetryBreaker: breaker,
//	})
func After(d time.Duration) Breaker {
	start := time.Now()
	return func(_ error) bool {
		return time.Since(start) > d
	}
}

// OnPanic stops attempts to run tasks when the last error originated by Go panic().
// This is useful when you want to stop retrying after a panic occurs, as panics
// typically indicate programming errors that won't be resolved by retrying.
//
// Example usage:
//
//	breaker := circuit.OnPanic()
//	observer.UseConfig(sentinel.ObserverConfig{
//		MaxRetries:  3,
//		RetryBreaker: breaker,
//	})
func OnPanic() Breaker {
	type panicError interface {
		error
		Value() any
	}
	return func(err error) bool {
		var target panicError
		return errors.As(err, &target)
	}
}
