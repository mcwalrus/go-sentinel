// Package circuit provides circuit breaker predicates for sentinel's retry logic.
package circuit

import (
	"errors"
	"time"
)

// Breaker determines whether retries should stop given the last error.
// It returns true to stop further retries.
type Breaker func(err error) bool

// OnPanic stops retries when the last error originated from a panic recovery.
// It detects sentinel.ErrPanicOccurred without importing the sentinel package
// by matching a type that satisfies the same method set.
func OnPanic() Breaker {
	type panicError interface {
		error
		PanicValue() any
	}
	return func(err error) bool {
		var target panicError
		return errors.As(err, &target)
	}
}

// After stops retries when total elapsed time since breaker creation exceeds d.
func After(d time.Duration) Breaker {
	start := time.Now()
	return func(err error) bool {
		_ = err
		return time.Since(start) > d
	}
}

// OnError uses a predicate to decide if retries should stop.
func OnError(match func(error) bool) Breaker {
	return func(err error) bool {
		if match == nil {
			return false
		}
		return match(err)
	}
}

// OnErrors stops on any of the specified errors (by errors.Is semantics).
func OnErrors(errs ...error) Breaker {
	return func(err error) bool {
		for _, e := range errs {
			if e != nil && errors.Is(err, e) {
				return true
			}
		}
		return false
	}
}

// OnErrorIs stops when errors.Is(err, target) is true.
func OnErrorIs(target error) Breaker {
	return func(err error) bool {
		return target != nil && errors.Is(err, target)
	}
}

// OnErrorAs stops when errors.As(err, &T) is true.
func OnErrorAs[T any]() Breaker {
	return func(err error) bool {
		var zero T
		return errors.As(err, &zero)
	}
}

// Any returns a breaker that stops when any of the provided breakers stop.
func Any(bs ...Breaker) Breaker {
	return func(err error) bool {
		for _, b := range bs {
			if b != nil && b(err) {
				return true
			}
		}
		return false
	}
}

// OnSignal stops when a value is received on the provided channel.
// The channel is read in a non-blocking way to keep the predicate fast.
func OnSignal[T any](ch <-chan T) Breaker {
	return func(err error) bool {
		_ = err
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
}

// OnDone stops when the provided done channel is closed.
func OnDone(done <-chan struct{}) Breaker {
	return func(err error) bool {
		_ = err
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
}
