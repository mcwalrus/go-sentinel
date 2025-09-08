package sentinel

import "time"

type RetryStrategy func(int) time.Duration

func RetryStrategyImmediate(retries int) time.Duration {
	return 0
}

func RetryStrategyLinearBackoff(timeToWait time.Duration) RetryStrategy {
	return func(retries int) time.Duration {
		return time.Duration(retries) * timeToWait
	}
}

func RetryStrategyExponentialBackoff(timeToWait time.Duration) RetryStrategy {
	return func(retries int) time.Duration {
		if retries == 0 {
			return 0
		}
		return time.Duration(1<<uint(retries-1)) * timeToWait
	}
}
