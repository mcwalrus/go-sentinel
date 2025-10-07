package sentinel

import (
	"testing"
	"time"
)

func TestRetryStrategyLinear(t *testing.T) {
	wait := 100 * time.Millisecond
	strategy := NewRetryStrategyLinear(wait)

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
				t.Errorf("RetryStrategy: LinearBackoff(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}
}

func TestRetryStrategyExponential(t *testing.T) {
	factor := 50 * time.Millisecond
	strategy := NewRetryStrategyExponential(factor)

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
					"RetryStrategy: ExponentialBackoff(%d) = %v, expected %v",
					tt.retries,
					result,
					tt.expected,
				)
			}
		})
	}
}
