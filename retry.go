package sentinel

import "time"

type RetryStrategy func(int) time.Duration

func RetryStrategyImmediate(retries int) time.Duration {
	return 0
}

func RetryStrategyExponential(retries int) time.Duration {
	return time.Duration(retries) * 100 * time.Millisecond
}

func RetryStrategyLinear(retries int) time.Duration {
	return time.Duration(retries) * 100 * time.Millisecond
}
