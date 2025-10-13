package sentinel

import (
	_ "context"
	"time"

	"github.com/mcwalrus/go-sentinel/circuit"
	"github.com/mcwalrus/go-sentinel/retry"
	"github.com/prometheus/client_golang/prometheus"
)

// ObserverOption defines prometheus metrics options for an [Observer].
// Options are provided to [NewObserver] on setting up the Observer.
type ObserverOption func(*config)

// config defines the configuration for a [Observer].
type config struct {
	namespace   string
	subsystem   string
	description string
	buckets     []float64
	constLabels prometheus.Labels
}

type ObserverConfig struct {
	// Timeout sets a context deadline tasks passed to [Observer.RunFunc].
	// The Observer records timeout occurrences via metrics when enabled.
	Timeout time.Duration

	// MaxRetries specifies the number of retry attempts for tasks on errors.
	// If a task fails for all attempts, the observer groups errors from multiple
	// attempts using [errors.Join]. By default, no retries are performed.
	MaxRetries int

	// RetryStrategy is a handler which returns wait durations between retry attempts.
	// The first call to the handler will provide retryCount at 0. Subsequent calls
	// will increment retryCount. By default, no wait strategy is applied.
	RetryStrategy retry.Strategy

	// RetryBreaker is a handler that skips following retry attempts for a task when
	// returning true. The handler will be provided the error from the previous attempt.
	// When nil, the observer will always attempt the next retry. This is useful to stop
	// retries on particular errors.
	RetryBreaker circuit.Breaker

	// Controls provides fine-grained control over observer behavior during different phases
	// of task execution. This is useful to manage or avoid new task executions on shutdown
	// signals, or through managed resource limits.
	Controls ObserverControls
}

type ObserverControls struct {
	// RequestControl prevents new task executions when returning true. This control is
	// checked before each call to Run() or RunFunc(). By default, requests are always allowed.
	RequestControl circuit.Control

	// InFlightControl prevents retry attempts when returning true. This control is checked
	// before each retry attempt for tasks that have failed. When nil, retries are allowed
	// according to the retry configuration.
	InFlightControl circuit.Control
}
