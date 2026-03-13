// Package retry provides WaitFunc implementations for the sentinel package.
// These strategies can be used with ObserverConfig.RetryStrategy to control retry behavior.
package retry

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"
)

// Retrier abstracts retry behavior. Implement this interface to provide custom
// retry strategies including backoff, jitter, or circuit-breaking.
type Retrier interface {
	Do(ctx context.Context, fn func() error) error
}

// DefaultRetrier is a configurable Retrier that integrates retry strategies with
// circuit-breaking. The Breaker field provides a stop condition based on the last
// error: when it returns true, retries halt immediately.
type DefaultRetrier struct {
	// WaitStrategy defines how long to wait between retry attempts.
	WaitStrategy WaitFunc

	// MaxRetries specifies the maximum number of retry attempts.
	MaxRetries int

	// Breaker is an optional function that stops retries when it returns true.
	// It receives the error from the previous attempt. When nil, retries continue
	// until MaxRetries is exhausted.
	Breaker func(err error) bool
}

// OnPanic returns a func(err error) bool that stops retries when the error
// originated from a Go panic. Use this as ObserverConfig.RetryBreaker to avoid
// retrying panic-induced failures.
func OnPanic() func(err error) bool {
	type panicError interface {
		error
		Value() any
	}
	return func(err error) bool {
		var target panicError
		return errors.As(err, &target)
	}
}

// WaitFunc defines a function to return wait durations from a specific retry
// attempt count. Retries start from 1 and increment with each call.
type WaitFunc func(retries int) time.Duration

// Immediate returns a WaitFunc that implements immediate retry with no delay.
func Immediate() WaitFunc {
	return func(_ int) time.Duration {
		return 0
	}
}

// Linear returns a WaitFunc that implements linear backoff.
// Each retry waits for wait * retryCount duration.
func Linear(wait time.Duration) WaitFunc {
	return func(retries int) time.Duration {
		if retries <= 0 {
			return 0
		}
		return time.Duration(retries) * wait
	}
}

// Exponential returns a WaitFunc that implements exponential backoff.
// Each retry waits for factor * 2^retryCount duration.
func Exponential(factor time.Duration) WaitFunc {
	return func(retries int) time.Duration {
		if retries <= 0 {
			return 0
		}
		return time.Duration(1<<uint(retries)) * factor
	}
}

// UseDelays returns a WaitFunc that implements a list of delays for each retry.
// The max delay will be returned for retries greater than the length of the delays.
//
// Example usage:
//
//	strategy := retry.UseDelays(
//		[]time.Duration{
//			100*time.Millisecond, 150*time.Millisecond,
//			250*time.Millisecond, 500*time.Millisecond,
//			1000*time.Millisecond, 3000*time.Millisecond,
//		},
//	)
func UseDelays(delays []time.Duration) WaitFunc {
	return func(retries int) time.Duration {
		if retries <= 0 {
			return 0
		}
		if retries >= len(delays) {
			return delays[len(delays)-1]
		}
		return delays[retries-1]
	}
}

// WithLimit wraps a WaitFunc to limit the maximum wait duration.
//
// Example usage:
//
//	strategy := retry.WithLimit(
//		1 * time.Second, // Limit to 1 second
//		retry.Exponential(100*time.Millisecond),
//	)
func WithLimit(limit time.Duration, strategy WaitFunc) WaitFunc {
	return func(retries int) time.Duration {
		wait := strategy(retries)
		if wait >= limit {
			return limit
		}
		return wait
	}
}

// ForgoAttempts returns a WaitFunc that skips the first n attempts.
//
// Example usage:
//
//	strategy := retry.ForgoAttempts(
//		2, // Skip the first 2 attempts
//		retry.Exponential(100*time.Millisecond),
//	)
func ForgoAttempts(skipN int, strategy WaitFunc) WaitFunc {
	return func(retries int) time.Duration {
		if retries-skipN <= 0 {
			return 0
		}
		return strategy(retries - skipN)
	}
}

// WithJitter wraps a WaitFunc with additional random jitter.
// The jitter is uniformly sampled in the range [0, jitter] and added to the base wait.
// This helps prevent thundering herd problems by spreading out retry attempts
// across multiple clients, reducing load spikes on recovering services.
//
// When jitter is less than or equal to zero, the base strategy is returned unchanged.
//
// Example usage:
//
//	strategy := retry.WithJitter(
//		time.Second, // Jitter up to 1 second
//		retry.Exponential(100*time.Millisecond),
//	)
func WithJitter(jitter time.Duration, strategy WaitFunc) WaitFunc {
	if jitter <= 0 {
		return strategy
	}
	return func(retries int) time.Duration {
		base := strategy(retries)
		maxNs := jitter.Nanoseconds()
		if maxNs <= 0 {
			return base
		}
		n, err := rand.Int(rand.Reader, big.NewInt(maxNs+1))
		if err != nil {
			return base
		}
		jitter := time.Duration(n.Int64())
		return base + jitter
	}
}
