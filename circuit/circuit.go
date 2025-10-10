// Package circuit provides circuit breaker predicates for sentinel's retry logic.
package circuit

import (
	"errors"
	"time"
)

// Breaker determines whether attempt to run task should stop given the latest error.
// It returns true to stop further attempts to run tasks. False will allow attempts to
// continue. The breaker will always expect an error to be provided.
type Breaker func(err error) bool

// Control determines whether attempt to run task should stop either before the initial
// attempt or any retry attempt. It returns true to stop further attempts to run tasks.
// False will allow attempts to continue.
type Control func() bool

// After stops retries when total elapsed time since breaker creation exceeds d.
func After(d time.Duration) Breaker {
	start := time.Now()
	return func(err error) bool {
		_ = err
		return time.Since(start) > d
	}
}

// OnMatch uses func match to decide if attempts to run tasks should stop.
// Note the approach can be achieved just by providing match as [Breaker] itself.
func OnMatch(match func(error) bool) Breaker {
	return func(err error) bool {
		if match == nil {
			return false
		}
		return match(err)
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
