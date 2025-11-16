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
// - Durations Histogram
// - Retry Attempts

type metrics struct {
	inFlight  prometheus.Gauge
	successes prometheus.Counter
	failures  prometheus.Counter
	errors    prometheus.Counter
	timeouts  prometheus.Counter
	panics    prometheus.Counter
	durations prometheus.Observer
	retries   prometheus.Counter
}

func newMetrics(cfg config) metrics {
	vecMetrics := newVecMetrics(cfg, nil)
	return vecMetrics.withLabels()
}

type vecMetrics struct {
	labelNames   []string
	inFlightVec  *prometheus.GaugeVec
	successesVec *prometheus.CounterVec
	failuresVec  *prometheus.CounterVec
	errorsVec    *prometheus.CounterVec
	timeoutsVec  *prometheus.CounterVec
	panicsVec    *prometheus.CounterVec
	durationsVec prometheus.ObserverVec
	retriesVec   *prometheus.CounterVec
}

func newVecMetrics(cfg config, labelNames []string) *vecMetrics {
	m := &vecMetrics{
		labelNames: labelNames,
		inFlightVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "in_flight",
			Help:        fmt.Sprintf("Number of observed %s in flight", cfg.description),
			ConstLabels: cfg.constLabels,
		}, labelNames),
		successesVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "success_total",
			Help:        fmt.Sprintf("Number of successes from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}, labelNames),
		failuresVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "failures_total",
			Help:        fmt.Sprintf("Number of failures from observed %s excluding retry attempts", cfg.description),
			ConstLabels: cfg.constLabels,
		}, labelNames),
		errorsVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "errors_total",
			Help:        fmt.Sprintf("Number of errors from observed %s including retries and panics", cfg.description),
			ConstLabels: cfg.constLabels,
		}, labelNames),
		panicsVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "panics_total",
			Help:        fmt.Sprintf("Number of panic occurrences from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}, labelNames),
		timeoutsVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "timeouts_total",
			Help:        fmt.Sprintf("Number of timeout errors from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}, labelNames),
		retriesVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "retries_total",
			Help:        fmt.Sprintf("Number of retry attempts from observed %s", cfg.description),
			ConstLabels: cfg.constLabels,
		}, labelNames),
	}

	if len(cfg.buckets) > 0 {
		m.durationsVec = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace:   cfg.namespace,
			Subsystem:   cfg.subsystem,
			Name:        "durations_seconds",
			Help:        fmt.Sprintf("Histogram of the observed durations of %s", cfg.description),
			Buckets:     cfg.buckets,
			ConstLabels: cfg.constLabels,
		}, labelNames)
	}

	return m
}

// metrics implementation

func (m *metrics) collectors() []prometheus.Collector {
	c := []prometheus.Collector{
		m.inFlight,
		m.successes,
		m.failures,
		m.errors,
		m.panics,
		m.timeouts,
		m.retries,
	}
	if m.durations != nil {
		if durations, ok := m.durations.(prometheus.Histogram); ok {
			c = append(c, durations)
		}
	}
	return c
}

func (m *metrics) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
}

func (m *metrics) Collect(ch chan<- prometheus.Metric) {
	for _, col := range m.collectors() {
		col.Collect(ch)
	}
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

// vecMetrics implementation

func (m *vecMetrics) collectors() []prometheus.Collector {
	c := []prometheus.Collector{
		m.inFlightVec,
		m.successesVec,
		m.failuresVec,
		m.errorsVec,
		m.panicsVec,
		m.timeoutsVec,
		m.retriesVec,
	}
	if m.durationsVec != nil {
		if durations, ok := (m.durationsVec).(*prometheus.HistogramVec); ok {
			c = append(c, durations)
		}
	}
	return c
}

func (m *vecMetrics) Describe(ch chan<- *prometheus.Desc) {
	prometheus.DescribeByCollect(m, ch)
}

func (m *vecMetrics) Collect(ch chan<- prometheus.Metric) {
	for _, col := range m.collectors() {
		col.Collect(ch)
	}
}

func (m *vecMetrics) MustRegister(registry prometheus.Registerer) {
	registry.MustRegister(m.collectors()...)
}

func (m *vecMetrics) Register(registry prometheus.Registerer) error {
	var errs []error
	for _, col := range m.collectors() {
		err := registry.Register(col)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *vecMetrics) with(labels prometheus.Labels) metrics {
	if len(labels) != len(m.labelNames) {
		panic("number of labels must match the number of label names")
	}
	ob := metrics{
		inFlight:  m.inFlightVec.With(labels),
		successes: m.successesVec.With(labels),
		failures:  m.failuresVec.With(labels),
		errors:    m.errorsVec.With(labels),
		panics:    m.panicsVec.With(labels),
		timeouts:  m.timeoutsVec.With(labels),
		retries:   m.retriesVec.With(labels),
	}
	if m.durationsVec != nil {
		ob.durations = m.durationsVec.With(labels)
	}

	return ob
}

func (m *vecMetrics) withLabels(labelValues ...string) metrics {
	if len(labelValues) != len(m.labelNames) {
		panic("number of label values must match the number of label names")
	}
	ob := metrics{
		inFlight:  m.inFlightVec.WithLabelValues(labelValues...),
		successes: m.successesVec.WithLabelValues(labelValues...),
		failures:  m.failuresVec.WithLabelValues(labelValues...),
		errors:    m.errorsVec.WithLabelValues(labelValues...),
		panics:    m.panicsVec.WithLabelValues(labelValues...),
		timeouts:  m.timeoutsVec.WithLabelValues(labelValues...),
		retries:   m.retriesVec.WithLabelValues(labelValues...),
	}
	if m.durationsVec != nil {
		ob.durations = m.durationsVec.WithLabelValues(labelValues...)
	}

	return ob
}

func (m *vecMetrics) CurryWith(labels prometheus.Labels) vecMetrics {
	if len(labels) >= len(m.labelNames) {
		panic("number of labels must be less than the number of label names")
	}
	ob := vecMetrics{
		inFlightVec:  m.inFlightVec.MustCurryWith(labels),
		successesVec: m.successesVec.MustCurryWith(labels),
		failuresVec:  m.failuresVec.MustCurryWith(labels),
		errorsVec:    m.errorsVec.MustCurryWith(labels),
		panicsVec:    m.panicsVec.MustCurryWith(labels),
		timeoutsVec:  m.timeoutsVec.MustCurryWith(labels),
		retriesVec:   m.retriesVec.MustCurryWith(labels),
	}
	if m.durationsVec != nil {
		ob.durationsVec = m.durationsVec.MustCurryWith(labels)
	}

	return ob
}
