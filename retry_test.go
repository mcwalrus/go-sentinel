package sentinel

import (
	"testing"
	"time"
)

func TestRetryStrategyImmediate(t *testing.T) {
	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{0, 0},
		{1, 0},
		{5, 0},
		{100, 0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := RetryStrategyImmediate(tt.retries)
			if result != tt.expected {
				t.Errorf(
					"RetryStrategyImmediate(%d) = %v, expected %v",
					tt.retries, result, tt.expected,
				)
			}
		})
	}
}

func TestRetryStrategyLinearBackoff(t *testing.T) {
	wait := 100 * time.Millisecond
	strategy := RetryStrategyLinearBackoff(wait)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{0, 0},
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{5, 500 * time.Millisecond},
		{10, 1000 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf("RetryS	trategyLinearBackoff(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}
}

func TestRetryStrategyExponentialBackoff(t *testing.T) {
	factor := 50 * time.Millisecond
	strategy := RetryStrategyExponentialBackoff(factor)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{0, 0},
		{1, 50 * time.Millisecond},
		{2, 100 * time.Millisecond},
		{3, 200 * time.Millisecond},
		{4, 400 * time.Millisecond},
		{5, 800 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf(
					"RetryStrategyExponentialBackoff(%d) = %v, expected %v",
					tt.retries,
					result,
					tt.expected,
				)
			}
		})
	}
}

func TestRetryStrategyExponentialBackoffWithLimit(t *testing.T) {
	factor := 50 * time.Millisecond
	limit := 300 * time.Millisecond
	strategy := RetryStrategyExponentialBackoffWithLimit(factor, limit)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{0, 0},
		{1, 50 * time.Millisecond},
		{2, 100 * time.Millisecond},
		{3, 200 * time.Millisecond},
		{4, 300 * time.Millisecond},
		{5, 300 * time.Millisecond},
		{10, 300 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf(
					"RetryStrategyExponentialBackoffWithLimit(%d) = %v, expected %v",
					tt.retries,
					result,
					tt.expected,
				)
			}
		})
	}
}
