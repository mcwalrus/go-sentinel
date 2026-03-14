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
// Deprecated: Use func(err error) bool directly in [sentinel.ObserverConfig.RetryBreaker].
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
// Deprecated: Use a plain func(err error) bool with time tracking in
// [retry.DefaultRetrier.Breaker] via [sentinel.WithRetrier].
//
// Example usage:
//
//	breaker := circuit.After(30 * time.Second)
//	observer := sentinel.NewObserver(sentinel.WithRetrier(retry.DefaultRetrier{
//		MaxRetries: 5,
//		Breaker:    breaker,
//	}))
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
// Deprecated: Use [retry.OnPanic] instead, which returns a plain func(err error) bool
// compatible with [retry.DefaultRetrier.Breaker] via [sentinel.WithRetrier].
//
// Example usage:
//
//	breaker := circuit.OnPanic()
//	observer := sentinel.NewObserver(sentinel.WithRetrier(retry.DefaultRetrier{
//		MaxRetries: 3,
//		Breaker:    breaker,
//	}))
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
