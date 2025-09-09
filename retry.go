package sentinel

import "time"

type RetryStrategy func(int) time.Duration

var (
	_ RetryStrategy = RetryStrategyImmediate
	_ RetryStrategy = RetryStrategyLinearBackoff(0)
	_ RetryStrategy = RetryStrategyExponentialBackoff(0)
	_ RetryStrategy = RetryStrategyExponentialBackoffWithLimit(0, 0)
)

func RetryStrategyImmediate(retries int) time.Duration {
	return 0
}

func RetryStrategyLinearBackoff(wait time.Duration) RetryStrategy {
	return func(retries int) time.Duration {
		return time.Duration(retries) * wait
	}
}

func RetryStrategyExponentialBackoff(factor time.Duration) RetryStrategy {
	return func(retries int) time.Duration {
		if retries == 0 {
			return 0
		} else {
			return time.Duration(1<<uint(retries-1)) * factor
		}
	}
}

func RetryStrategyExponentialBackoffWithLimit(factor time.Duration, limit time.Duration) RetryStrategy {
	return func(retries int) time.Duration {
		result := RetryStrategyExponentialBackoff(factor)(retries)
		if result > limit {
			return limit
		} else {
			return result
		}
	}
}
