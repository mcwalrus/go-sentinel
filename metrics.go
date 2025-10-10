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
	FinalFailures    prometheus.Counter
}

// newMetrics creates a new metrics instance with the given configuration.
// If bucketDurations is provided, the ObservedRuntimes histogram will be created.
func newMetrics(cfg observerConfig) *metrics {
	m := &metrics{
		InFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "in_flight",
			Help:        fmt.Sprintf("Number of observed %s in flight", cfg.description),
			ConstLabels: cfg.constLabels,
		}),
		Successes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "success_total",
			Help:        fmt.Sprintf("Number of successes from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}),
		Errors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "errors_total",
			Help:        fmt.Sprintf("Number of errors from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}),
		Panics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "panics_total",
			Help:        fmt.Sprintf("Number of panic occurances from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}),
	}

	if len(cfg.bucketDurations) > 0 {
		m.ObservedRuntimes = prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "durations_seconds",
			Help:        fmt.Sprintf("Histogram of the observed durations of %s", cfg.description),
			Buckets:     cfg.bucketDurations,
			ConstLabels: cfg.constLabels,
		})
	}

	if cfg.trackTimeouts {
		m.TimeoutErrors = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "timeouts_total",
			Help:        fmt.Sprintf("Number of timeout errors from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		})
	}

	if cfg.trackRetries {
		m.Retries = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "retries_total",
			Help:        fmt.Sprintf("Number of retry attempts from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		})
		m.FinalFailures = prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "failures_total",
			Help:        fmt.Sprintf("Number of final failures from observed %s after all retry attempts", cfg.description),
			ConstLabels: cfg.constLabels,
		})
	}

	return m
}

// collectors returns all collectors for the metrics.
func (m *metrics) collectors() []prometheus.Collector {
	c := []prometheus.Collector{
		m.InFlight,
		m.Successes,
		m.Errors,
		m.Panics,
	}
	if m.TimeoutErrors != nil {
		c = append(c, m.TimeoutErrors)
	}
	if m.ObservedRuntimes != nil {
		c = append(c, m.ObservedRuntimes)
	}
	if m.Retries != nil {
		c = append(c, m.Retries)
	}
	if m.FinalFailures != nil {
		c = append(c, m.FinalFailures)
	}
	return c
}

// MustRegister registers all metrics with the given registry.
// It panics if any metric fails to register.
func (m *metrics) MustRegister(registry prometheus.Registerer) {
	registry.MustRegister(m.collectors()...)
}

// Register registers all metrics with the given registry.
// It returns an error if any metric fails to register.
func (m *metrics) Register(registry prometheus.Registerer) error {
	var errs []error
	for _, col := range m.collectors() {
		err := registry.Register(col)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
