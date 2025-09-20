package sentinel

import (
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Promtheus metrics:

// - In Flight
// - Successes
// - Error count
// - Timeout Errors
// - Panics Occurances
// - Routine Runtime Histogram
// - Retry Attempts

type metrics struct {
	InFlight         prometheus.Gauge
	Successes        prometheus.Counter
	Errors           prometheus.Counter
	TimeoutErrors    prometheus.Counter
	Panics           prometheus.Counter
	ObservedRuntimes prometheus.Histogram
	Retries          prometheus.Counter
}

// newMetrics creates a new metrics instance with the given configuration.
func newMetrics(cfg ObserverConfig) *metrics {
	return &metrics{
		InFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   cfg.Namespace,
			Subsystem:   cfg.Subsystem,
			Name:        "in_flight",
			Help:        fmt.Sprintf("Number of observed %s in flight", cfg.Description),
			ConstLabels: cfg.ConstLabels,
		}),
		Successes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.Namespace,
			Subsystem:   cfg.Subsystem,
			Name:        "successes_total",
			Help:        fmt.Sprintf("Number of successes from observed %s", cfg.Description),
			ConstLabels: cfg.ConstLabels,
		}),
		Errors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.Namespace,
			Subsystem:   cfg.Subsystem,
			Name:        "errors_total",
			Help:        fmt.Sprintf("Number of errors from observed %s", cfg.Description),
			ConstLabels: cfg.ConstLabels,
		}),
		TimeoutErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.Namespace,
			Subsystem:   cfg.Subsystem,
			Name:        "timeouts_total",
			Help:        fmt.Sprintf("Number of timeout errors from observed %s", cfg.Description),
			ConstLabels: cfg.ConstLabels,
		}),
		Panics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.Namespace,
			Subsystem:   cfg.Subsystem,
			Name:        "panics_total",
			Help:        fmt.Sprintf("Number of panic occurances from observed %s", cfg.Description),
			ConstLabels: cfg.ConstLabels,
		}),
		ObservedRuntimes: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   cfg.Namespace,
			Subsystem:   cfg.Subsystem,
			Name:        "observed_duration_seconds",
			Help:        fmt.Sprintf("Histogram of the observed durations of %s", cfg.Description),
			Buckets:     cfg.BucketDurations,
			ConstLabels: cfg.ConstLabels,
		}),
		Retries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.Namespace,
			Subsystem:   cfg.Subsystem,
			Name:        "retries_total",
			Help:        fmt.Sprintf("Number of retry attempts from observed %s", cfg.Description),
			ConstLabels: cfg.ConstLabels,
		}),
	}
}

// MustRegister registers all metrics with the given registry.
// It panics if any metric fails to register.
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

// Register registers all metrics with the given registry.
// It returns an error if any metric fails to register.
func (m *metrics) Register(registry prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		m.Successes,
		m.Errors,
		m.TimeoutErrors,
		m.Panics,
		m.ObservedRuntimes,
		m.Retries,
	}
	var errs []error
	for _, col := range collectors {
		err := registry.Register(col)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
