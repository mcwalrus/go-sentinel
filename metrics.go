package sentinel

import (
	"errors"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
)

// Prometheus metrics:

// - In Flight
// - Successes
// - Failures
// - Error count
// - Timeout Errors
// - Panics Occurrences
// - Routine Runtime Histogram
// - Retry Attempts

type metrics struct {
	InFlight  prometheus.Gauge
	Successes prometheus.Counter
	Failures  prometheus.Counter
	Errors    prometheus.Counter
	Timeouts  prometheus.Counter
	Panics    prometheus.Counter
	Durations prometheus.Histogram
	Retries   prometheus.Counter
}

func newMetrics(cfg config) *metrics {
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
		Failures: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "failures_total",
			Help:        fmt.Sprintf("Number of failures from observed %s excluding retry attempts", cfg.description),
			ConstLabels: cfg.constLabels,
		}),
		Errors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "errors_total",
			Help:        fmt.Sprintf("Number of errors from observed %s including retries and panics", cfg.description),
			ConstLabels: cfg.constLabels,
		}),
		Panics: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "panics_total",
			Help:        fmt.Sprintf("Number of panic occurrences from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}),
	}

	if len(cfg.buckets) > 0 {
		m.Durations = prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "durations_seconds",
			Help:        fmt.Sprintf("Histogram of the observed durations of %s", cfg.description),
			Buckets:     cfg.buckets,
			ConstLabels: cfg.constLabels,
		})
	}

	if cfg.trackTimeouts {
		m.Timeouts = prometheus.NewCounter(prometheus.CounterOpts{
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
	}

	return m
}

func (m *metrics) collectors() []prometheus.Collector {
	c := []prometheus.Collector{
		m.InFlight,
		m.Successes,
		m.Failures,
		m.Errors,
		m.Panics,
	}
	if m.Timeouts != nil {
		c = append(c, m.Timeouts)
	}
	if m.Durations != nil {
		c = append(c, m.Durations)
	}
	if m.Retries != nil {
		c = append(c, m.Retries)
	}
	return c
}

func (m *metrics) MustRegister(registry prometheus.Registerer) {
	registry.MustRegister(m.collectors()...)
}

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
