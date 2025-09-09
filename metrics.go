package sentinel

import (
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Promthesus metrics:

// - In Flight
// - Successes
// - Error count
// - Timeout Errors
// - Panics Occurances
// - Routine Runtime Histogram
// - Retries

type metrics struct {
	InFlight         prometheus.Gauge
	Successes        prometheus.Counter
	Errors           prometheus.Counter
	TimeoutErrors    prometheus.Counter
	Panics           prometheus.Counter
	ObservedRuntimes prometheus.Histogram
	Retries          prometheus.Counter
}

func newMetrics(cfg ObserverConfig) *metrics {
	return &metrics{
		InFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "in_flight",
			Help:      fmt.Sprintf("Number of observed %s tasks in flight", cfg.Description),
		}),
		Successes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "successes",
			Help:      fmt.Sprintf("Number of successes from observed %s tasks", cfg.Description),
		}),
		Errors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "errors",
			Help:      fmt.Sprintf("Number of errors from observed %s tasks", cfg.Description),
		}),
		TimeoutErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "timeout_errors",
			Help:      fmt.Sprintf("Number of timeout errors from observed %s tasks", cfg.Description),
		}),
		Panics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "panic_occurances",
			Help:      fmt.Sprintf("Number of panics from observed %s tasks", cfg.Description),
		}),
		ObservedRuntimes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "observed_duration",
			Help:      fmt.Sprintf("Runtime of observed %s tasks", cfg.Description),
			Buckets:   cfg.BucketDurations,
		}),
		Retries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Subsystem: cfg.Subsystem,
			Name:      "retry_attempts",
			Help:      fmt.Sprintf("Number of retries from observed %s tasks", cfg.Description),
		}),
	}
}

func (m *metrics) MustRegister(registry prometheus.Registerer) {
	registry.MustRegister(
		m.InFlight,
		m.Successes,
		m.Errors,
		m.TimeoutErrors,
		m.Panics,
		m.ObservedRuntimes,
		m.Retries,
	)
}

func (m *metrics) Register(registry prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		m.Successes,
		m.Errors,
		m.TimeoutErrors,
		m.Panics,
		m.ObservedRuntimes,
		m.Retries,
	}
	var joinErrs error
	for _, col := range collectors {
		err := registry.Register(col)
		if err != nil {
			joinErrs = errors.Join(joinErrs, err)
		}
	}
	return joinErrs
}
