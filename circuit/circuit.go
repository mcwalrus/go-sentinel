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
//	breaker := func(err error) bool {
//		return errors.Is(err, context.Canceled)
//	}
type Breaker func(err error) bool

// After stops retries when total elapsed time since breaker creation exceeds d.
func After(d time.Duration) Breaker {
	start := time.Now()
	return func(err error) bool {
		_ = err
		return time.Since(start) > d
	}
}

// OnPanic stops attempts to run tasks when the last error originated by Go panic().
// It can detect sentinel.ErrRecoveredPanic without importing the sentinel package
// by matching a interface that satisfies the same method set.
func OnPanic() Breaker {
	type panicError interface {
		error
		RecoveredPanic() any
	}
	return func(err error) bool {
		var target panicError
		return errors.As(err, &target)
	}
}

// Any returns a breaker that stops when any of the provided breakers match.
func AnyBreaker(bs ...Breaker) Breaker {
	return func(err error) bool {
		for _, b := range bs {
			if b != nil && b(err) {
				return true
			}
		}
		return false
	}
}
