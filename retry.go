package sentinel

import "time"

// RetryStrategy defines a function that calculates the wait duration before a retry attempt.
// The parameter represents the number of retries that have already been attempted (0 for first retry).
// It returns the duration to wait before the next retry attempt.
type RetryStrategy func(int) time.Duration

var (
	_ RetryStrategy = RetryStrategyImmediate
	_ RetryStrategy = RetryStrategyLinearBackoff(0)
	_ RetryStrategy = RetryStrategyExponentialBackoff(0)
)

// RetryStrategyImmediate is a retry strategy that performs immediately with no delay.
func RetryStrategyImmediate(retries int) time.Duration {
	return 0
}

// RetryStrategyLinearBackoff returns a retry strategy that implements linear backoff.
func RetryStrategyLinearBackoff(wait time.Duration) RetryStrategy {
	return func(retries int) time.Duration {
		return time.Duration(retries) * wait
	}
}

// RetryStrategyExponentialBackoff returns a retry strategy that implements exponential backoff.
func RetryStrategyExponentialBackoff(factor time.Duration) RetryStrategy {
	return func(retries int) time.Duration {
		if retries == 0 {
			return 0
		} else {
			return time.Duration(1<<uint(retries-1)) * factor
		}
	}
}
