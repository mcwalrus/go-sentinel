package retry

import (
	"testing"
	"time"
)

func TestImmediate(t *testing.T) {
	t.Parallel()

	strategy := Immediate()

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{1, 0},
		{2, 0},
		{5, 0},
		{10, 0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf("Immediate(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}
}

func TestLinear(t *testing.T) {
	t.Parallel()

	wait := 100 * time.Millisecond
	strategy := Linear(wait)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{1, 200 * time.Millisecond},
		{2, 300 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{5, 600 * time.Millisecond},
		{10, 1100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf("Linear(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}
}

func TestExponential(t *testing.T) {
	t.Parallel()

	factor := 50 * time.Millisecond
	strategy := Exponential(factor)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{0, 0},
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf("Exponential(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}
}

func TestUseDelays(t *testing.T) {
	t.Parallel()

	delays := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond}
	strategy := UseDelays(delays)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 300 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf("UseDelays(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}
}

func TestWithLimit(t *testing.T) {
	t.Parallel()

	baseStrategy := Linear(100 * time.Millisecond)
	limit := 300 * time.Millisecond
	strategy := WithLimit(limit, baseStrategy)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{0, 100 * time.Millisecond}, // under limit
		{1, 200 * time.Millisecond}, // under limit
		{2, 300 * time.Millisecond}, // at limit
		{3, 300 * time.Millisecond}, // over limit, capped
		{5, 300 * time.Millisecond}, // over limit, capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf("WithLimit(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}
}

func TestWithJitter(t *testing.T) {
	t.Parallel()

	baseStrategy := Linear(100 * time.Millisecond)
	maxJitter := 50 * time.Millisecond
	strategy := WithJitter(maxJitter, baseStrategy)

	// Test that jitter is within expected range
	for retries := 0; retries < 5; retries++ {
		base := baseStrategy(retries)
		result := strategy(retries)

		// Result should be base + jitter, where jitter is in [0, maxJitter]
		if result < base || result > base+maxJitter {
			t.Errorf("WithJitter(%d) = %v, expected to be in range [%v, %v]",
				retries, result, base, base+maxJitter)
		}
	}

	// Test with zero jitter
	zeroJitterStrategy := WithJitter(0, baseStrategy)
	for retries := 0; retries < 3; retries++ {
		expected := baseStrategy(retries)
		result := zeroJitterStrategy(retries)
		if result != expected {
			t.Errorf("WithJitter(zero)(%d) = %v, expected %v", retries, result, expected)
		}
	}

	// Test with negative jitter
	negativeJitterStrategy := WithJitter(-10*time.Millisecond, baseStrategy)
	for retries := 0; retries < 3; retries++ {
		expected := baseStrategy(retries)
		result := negativeJitterStrategy(retries)
		if result != expected {
			t.Errorf("WithJitter(negative)(%d) = %v, expected %v", retries, result, expected)
		}
	}
}

func TestComposition(t *testing.T) {
	t.Parallel()

	// Test composing multiple strategies: Linear -> WithLimit -> WithJitter
	baseStrategy := Linear(100 * time.Millisecond)
	limitedStrategy := WithLimit(300*time.Millisecond, baseStrategy)
	finalStrategy := WithJitter(50*time.Millisecond, limitedStrategy)

	// Test that composition works correctly
	for retries := 0; retries < 5; retries++ {
		base := baseStrategy(retries)
		limited := limitedStrategy(retries)
		result := finalStrategy(retries)

		// Limited should cap the base
		expectedLimited := base
		if base >= 300*time.Millisecond {
			expectedLimited = 300 * time.Millisecond
		}
		if limited != expectedLimited {
			t.Errorf("WithLimit(%d) = %v, expected %v", retries, limited, expectedLimited)
		}

		// Final result should be limited + jitter
		if result < limited || result > limited+50*time.Millisecond {
			t.Errorf("Composed strategy(%d) = %v, expected to be in range [%v, %v]",
				retries, result, limited, limited+50*time.Millisecond)
		}
	}
}

func TestForgoAttempts(t *testing.T) {
	t.Parallel()

	baseStrategy := Linear(100 * time.Millisecond)
	skipN := 2
	strategy := ForgoAttempts(skipN, baseStrategy)

	tests := []struct {
		retries  int
		expected time.Duration
	}{
		{1, 0},
		{2, 0},
		{3, 200 * time.Millisecond},
		{4, 300 * time.Millisecond},
		{5, 400 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			result := strategy(tt.retries)
			if result != tt.expected {
				t.Errorf("ForgoAttempts(skipN=%d)(%d) = %v, expected %v", skipN, tt.retries, result, tt.expected)
			}
		})
	}

	// Test with Exponential strategy
	expStrategy := Exponential(50 * time.Millisecond)
	skipExpStrategy := ForgoAttempts(1, expStrategy)

	expTests := []struct {
		retries  int
		expected time.Duration
	}{
		{1, 0},
		{2, 100 * time.Millisecond},
		{3, 200 * time.Millisecond},
		{4, 400 * time.Millisecond},
	}

	for _, tt := range expTests {
		t.Run("", func(t *testing.T) {
			t.Parallel()

			result := skipExpStrategy(tt.retries)
			if result != tt.expected {
				t.Errorf("ForgoAttempts(skipN=1, Exponential)(%d) = %v, expected %v", tt.retries, result, tt.expected)
			}
		})
	}

	// Test with skipN=0 (should behave like base strategy)
	noSkipStrategy := ForgoAttempts(0, baseStrategy)
	for retries := 1; retries <= 3; retries++ {
		expected := baseStrategy(retries)
		result := noSkipStrategy(retries)
		if result != expected {
			t.Errorf("ForgoAttempts(skipN=0)(%d) = %v, expected %v", retries, result, expected)
		}
	}

	// Test with Immediate strategy (should always return 0)
	immediateStrategy := Immediate()
	skipImmediateStrategy := ForgoAttempts(5, immediateStrategy)
	for retries := 1; retries <= 10; retries++ {
		result := skipImmediateStrategy(retries)
		if result != 0 {
			t.Errorf("ForgoAttempts(Immediate)(%d) = %v, expected 0", retries, result)
		}
	}
}
