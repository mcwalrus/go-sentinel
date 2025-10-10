// Package retry provides retry strategy implementations for the sentinel package.
// These strategies can be used with TaskConfig.RetryStrategy to control retry behavior.
package retry

import (
	"crypto/rand"
	"math/big"
	"time"
)

// Strategy defines a retry strategy function that returns the wait duration
// for a given retry attempt count.
type Strategy func(retryCount int) time.Duration

// Immediate returns a retry strategy that implements immediate retry with no delay.
func Immediate() Strategy {
	return func(retries int) time.Duration {
		return 0
	}
}

// Linear returns a retry strategy that implements linear backoff.
// The wait duration increases linearly with each retry: wait * (retryCount + 1).
func Linear(wait time.Duration) Strategy {
	return func(retries int) time.Duration {
		return time.Duration(retries+1) * wait
	}
}

// Exponential returns a retry strategy that implements exponential backoff.
// The duration is the base factor for the exponential backoff where the first retry
// will wait for duration, second retry for 2*duration, third for 4*duration, etc.
func Exponential(factor time.Duration) Strategy {
	return func(retries int) time.Duration {
		if retries == 0 {
			return 0
		}
		return time.Duration(1<<uint(retries)) * factor
	}
}

// WithLimit wraps a retry strategy to limit the maximum wait duration.
// If the base strategy returns a duration greater than or equal to the limit,
// the limit is returned instead.
func WithLimit(strategy Strategy, limit time.Duration) Strategy {
	return func(retries int) time.Duration {
		wait := strategy(retries)
		if wait >= limit {
			return limit
		}
		return wait
	}
}

// WithJitter wraps a retry strategy with additional random jitter.
// The jitter is uniformly sampled in the range [0, maxJitter] and added to the base wait.
// When maxJitter is less than or equal to zero, the base strategy is returned unchanged.
func WithJitter(strategy Strategy, maxJitter time.Duration) Strategy {
	if maxJitter <= 0 {
		return strategy
	}

	return func(retries int) time.Duration {
		base := strategy(retries)

		maxNs := maxJitter.Nanoseconds()
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
