package sentinel

import (
	"crypto/rand"
	"math/big"
	"time"
)

// NewRetryStrategyImmediate returns a retry strategy that implements immediate retry.
func NewRetryStrategyImmediate() func(int) time.Duration {
	return func(retries int) time.Duration {
		return 0
	}
}

// NewRetryStrategyLinear returns a retry strategy that implements linear backoff.
func NewRetryStrategyLinear(wait time.Duration) func(int) time.Duration {
	return func(retries int) time.Duration {
		return time.Duration(retries+1) * wait
	}
}

// NewRetryStrategyExponential returns a retry strategy that implements exponential backoff.
// The duration is the base factor for the exponential backoff where the first retry will wait
// for duration.
func NewRetryStrategyExponential(factor time.Duration) func(int) time.Duration {
	return func(retries int) time.Duration {
		if retries == 0 {
			return 0
		} else {
			// return time.Duration(1<<uint(retries-1)) * factor
			return time.Duration(1<<uint(retries)) * factor
		}
	}
}

// RetryStrategyWithLimit returns a retry strategy that limits the maximum wait duration.
func RetryStrategyWithLimit(strategy func(int) time.Duration, limit time.Duration) func(int) time.Duration {
	return func(retries int) time.Duration {
		if wait := strategy(retries); wait >= limit {
			return limit
		} else {
			return wait
		}
	}
}

// RetryStrategyWithJitter wraps an existing retry strategy with additional random jitter.
// The jitter is uniformly sampled in the range [0, maxJitter] plus the base wait duration.
// When maxJitter is less than or equal to zero, the base strategy will be returned unchanged.
func RetryStrategyWithJitter(strategy func(int) time.Duration, maxJitter time.Duration) func(int) time.Duration {
	if maxJitter <= 0 {
		return strategy
	}
	return func(retries int) time.Duration {
		var base time.Duration
		if strategy != nil {
			base = strategy(retries)
		}

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
