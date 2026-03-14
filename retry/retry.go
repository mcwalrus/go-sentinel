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
// retry strategies including backoff, jitter, or circuit-breaking. Use
// [DefaultRetrier] for the standard implementation, or implement Retrier
// directly when you need full control over the retry loop (e.g., integrating
// with a third-party circuit breaker or distributed tracing).
type Retrier interface {
	// Do executes fn and retries on error until the strategy is exhausted or
	// ctx is cancelled. It returns nil on the first successful call, or a
	// joined error from all attempts if every call fails.
	Do(ctx context.Context, fn func() error) error
}

// DefaultRetrier is a configurable [Retrier] that integrates retry strategies
// with circuit-breaking. Construct one with [NewDefaultRetrier] or by
// initialising the struct directly. The Breaker field provides an optional
// stop condition: when it returns true for the last error, retries halt
// immediately rather than waiting for MaxRetries to be exhausted.
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

// Do executes fn, retrying on error up to MaxRetries times using the configured
// WaitStrategy and Breaker. Returns nil on first success, or the joined errors
// from all attempts if all fail. Respects ctx cancellation between retries.
//
// Panics in WaitStrategy or Breaker are recovered silently: a panicking Breaker
// is treated as returning false (retries continue), and a panicking WaitStrategy
// is treated as returning 0 (immediate retry).
func (r DefaultRetrier) Do(ctx context.Context, fn func() error) error {
	var errs []error
	for attempt := 0; attempt <= r.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		err := fn()
		if err == nil {
			return nil
		}
		errs = append(errs, err)
		if attempt >= r.MaxRetries {
			break
		}
		if r.Breaker != nil && safeBreaker(r.Breaker, err) {
			break
		}
		if r.WaitStrategy != nil {
			wait := safeWait(r.WaitStrategy, attempt+1)
			if wait > 0 {
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return errors.Join(append(errs, ctx.Err())...)
				}
			}
		}
	}
	return errors.Join(errs...)
}

// safeBreaker calls breaker(err) and recovers any panic, returning true on panic.
// A panicking breaker defaults to stopping retries, consistent with the principle
// that a broken breaker should fail safe (halt rather than continue indefinitely).
func safeBreaker(breaker func(err error) bool, err error) (stop bool) {
	defer func() {
		if r := recover(); r != nil {
			stop = true
		}
	}()
	return breaker(err)
}

// safeWait calls strategy(attempt) and recovers any panic, returning 0 on panic.
func safeWait(strategy WaitFunc, attempt int) (wait time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			wait = 0
		}
	}()
	return strategy(attempt)
}

// NewDefaultRetrier creates a [DefaultRetrier] with the given strategy and
// maxRetries. Pass nil for strategy to use immediate retry with no wait
// between attempts.
//
// Example usage:
//
//	retrier := retry.NewDefaultRetrier(
//		retry.WithLimit(2*time.Second, retry.Exponential(100*time.Millisecond)),
//		5,
//	)
func NewDefaultRetrier(strategy WaitFunc, maxRetries int) *DefaultRetrier {
	return &DefaultRetrier{
		WaitStrategy: strategy,
		MaxRetries:   maxRetries,
	}
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

// WaitFunc defines a function that returns the wait duration before a given
// retry attempt. The retries argument starts at 1 for the first retry and
// increments with each subsequent call. Returning 0 means no delay before the
// next attempt.
type WaitFunc func(retries int) time.Duration

// Immediate returns a WaitFunc that retries without any delay between attempts.
// Use this when the operation is idempotent and low-latency retry is acceptable,
// such as in tests or when the error is expected to clear instantly.
//
// Example usage:
//
//	retrier := retry.NewDefaultRetrier(retry.Immediate(), 3)
func Immediate() WaitFunc {
	return func(_ int) time.Duration {
		return 0
	}
}

// Linear returns a WaitFunc that implements linear backoff. The wait duration
// grows proportionally with the retry count: wait * retryCount. Returns 0 when
// retryCount is zero or negative.
//
// Example usage:
//
//	strategy := retry.Linear(200 * time.Millisecond) // 200ms, 400ms, 600ms, ...
func Linear(wait time.Duration) WaitFunc {
	return func(retries int) time.Duration {
		if retries <= 0 {
			return 0
		}
		return time.Duration(retries) * wait
	}
}

// Exponential returns a WaitFunc that implements exponential backoff. The wait
// duration doubles with every retry: factor * 2^retryCount. Returns 0 when
// retryCount is zero or negative. Combine with [WithLimit] to cap the maximum
// wait and [WithJitter] to avoid thundering herd problems.
//
// Example usage:
//
//	strategy := retry.Exponential(100 * time.Millisecond) // 200ms, 400ms, 800ms, ...
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
