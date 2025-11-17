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
	return vecMetrics.withLabelsMust()
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

func (m *vecMetrics) withMust(labels prometheus.Labels) metrics {
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

func (m *vecMetrics) withLabelsMust(labelValues ...string) metrics {
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

func (m *vecMetrics) with(labels prometheus.Labels) (metrics, error) {
	if len(labels) != len(m.labelNames) {
		return metrics{}, fmt.Errorf("number of labels (%d) must match the number of label names (%d)", len(labels), len(m.labelNames))
	}

	inFlight, err := m.inFlightVec.GetMetricWith(labels)
	if err != nil {
		return metrics{}, err
	}
	successes, err := m.successesVec.GetMetricWith(labels)
	if err != nil {
		return metrics{}, err
	}
	failures, err := m.failuresVec.GetMetricWith(labels)
	if err != nil {
		return metrics{}, err
	}
	errorsMetric, err := m.errorsVec.GetMetricWith(labels)
	if err != nil {
		return metrics{}, err
	}
	panics, err := m.panicsVec.GetMetricWith(labels)
	if err != nil {
		return metrics{}, err
	}
	timeouts, err := m.timeoutsVec.GetMetricWith(labels)
	if err != nil {
		return metrics{}, err
	}
	retries, err := m.retriesVec.GetMetricWith(labels)
	if err != nil {
		return metrics{}, err
	}

	ob := metrics{
		inFlight:  inFlight,
		successes: successes,
		failures:  failures,
		errors:    errorsMetric,
		panics:    panics,
		timeouts:  timeouts,
		retries:   retries,
	}
	if m.durationsVec != nil {
		durations, err := m.durationsVec.GetMetricWith(labels)
		if err != nil {
			return metrics{}, err
		}
		ob.durations = durations
	}

	return ob, nil
}

func (m *vecMetrics) withLabelValues(labelValues ...string) (metrics, error) {
	if len(labelValues) != len(m.labelNames) {
		return metrics{}, fmt.Errorf("number of label values (%d) must match the number of label names (%d)", len(labelValues), len(m.labelNames))
	}

	inFlight, err := m.inFlightVec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return metrics{}, err
	}
	successes, err := m.successesVec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return metrics{}, err
	}
	failures, err := m.failuresVec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return metrics{}, err
	}
	errorsMetric, err := m.errorsVec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return metrics{}, err
	}
	panics, err := m.panicsVec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return metrics{}, err
	}
	timeouts, err := m.timeoutsVec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return metrics{}, err
	}
	retries, err := m.retriesVec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		return metrics{}, err
	}

	ob := metrics{
		inFlight:  inFlight,
		successes: successes,
		failures:  failures,
		errors:    errorsMetric,
		panics:    panics,
		timeouts:  timeouts,
		retries:   retries,
	}
	if m.durationsVec != nil {
		durations, err := m.durationsVec.GetMetricWithLabelValues(labelValues...)
		if err != nil {
			return metrics{}, err
		}
		ob.durations = durations
	}

	return ob, nil
}

func (m *vecMetrics) CurryWith(labels prometheus.Labels) (vecMetrics, error) {
	if len(labels) >= len(m.labelNames) {
		return vecMetrics{}, fmt.Errorf("number of labels (%d) must be less than the number of label names (%d)", len(labels), len(m.labelNames))
	}

	// Compute remaining label names after currying
	curriedLabelNames := make([]string, 0, len(m.labelNames)-len(labels))
	curriedLabelsSet := make(map[string]bool, len(labels))
	for k := range labels {
		curriedLabelsSet[k] = true
	}
	for _, name := range m.labelNames {
		if !curriedLabelsSet[name] {
			curriedLabelNames = append(curriedLabelNames, name)
		}
	}

	curriedInFlight, err := m.inFlightVec.CurryWith(labels)
	if err != nil {
		return vecMetrics{}, err
	}
	curriedSuccesses, err := m.successesVec.CurryWith(labels)
	if err != nil {
		return vecMetrics{}, err
	}
	curriedFailures, err := m.failuresVec.CurryWith(labels)
	if err != nil {
		return vecMetrics{}, err
	}
	curriedErrors, err := m.errorsVec.CurryWith(labels)
	if err != nil {
		return vecMetrics{}, err
	}
	curriedPanics, err := m.panicsVec.CurryWith(labels)
	if err != nil {
		return vecMetrics{}, err
	}
	curriedTimeouts, err := m.timeoutsVec.CurryWith(labels)
	if err != nil {
		return vecMetrics{}, err
	}
	curriedRetries, err := m.retriesVec.CurryWith(labels)
	if err != nil {
		return vecMetrics{}, err
	}

	ob := vecMetrics{
		labelNames:   curriedLabelNames,
		inFlightVec:  curriedInFlight,
		successesVec: curriedSuccesses,
		failuresVec:  curriedFailures,
		errorsVec:    curriedErrors,
		panicsVec:    curriedPanics,
		timeoutsVec:  curriedTimeouts,
		retriesVec:   curriedRetries,
	}
	if m.durationsVec != nil {
		curriedDurations, err := m.durationsVec.CurryWith(labels)
		if err != nil {
			return vecMetrics{}, err
		}
		ob.durationsVec = curriedDurations
	}

	return ob, nil
}

// Delete deletes the metrics where the variable labels match the provided labels.
// It returns true if any metrics were deleted.
func (m *vecMetrics) Delete(labels prometheus.Labels) bool {
	if len(labels) != len(m.labelNames) {
		return false
	}

	deleted := false
	deleted = m.inFlightVec.Delete(labels) || deleted
	deleted = m.successesVec.Delete(labels) || deleted
	deleted = m.failuresVec.Delete(labels) || deleted
	deleted = m.errorsVec.Delete(labels) || deleted
	deleted = m.panicsVec.Delete(labels) || deleted
	deleted = m.timeoutsVec.Delete(labels) || deleted
	deleted = m.retriesVec.Delete(labels) || deleted

	if m.durationsVec != nil {
		if durationsVec, ok := m.durationsVec.(*prometheus.HistogramVec); ok {
			deleted = durationsVec.Delete(labels) || deleted
		}
	}

	return deleted
}

// DeleteLabelValues deletes the metrics where the variable labels match the provided label values.
// It returns true if any metrics were deleted.
func (m *vecMetrics) DeleteLabelValues(labelValues ...string) bool {
	if len(labelValues) != len(m.labelNames) {
		return false
	}

	deleted := false
	deleted = m.inFlightVec.DeleteLabelValues(labelValues...) || deleted
	deleted = m.successesVec.DeleteLabelValues(labelValues...) || deleted
	deleted = m.failuresVec.DeleteLabelValues(labelValues...) || deleted
	deleted = m.errorsVec.DeleteLabelValues(labelValues...) || deleted
	deleted = m.panicsVec.DeleteLabelValues(labelValues...) || deleted
	deleted = m.timeoutsVec.DeleteLabelValues(labelValues...) || deleted
	deleted = m.retriesVec.DeleteLabelValues(labelValues...) || deleted

	if m.durationsVec != nil {
		if durationsVec, ok := m.durationsVec.(*prometheus.HistogramVec); ok {
			deleted = durationsVec.DeleteLabelValues(labelValues...) || deleted
		}
	}

	return deleted
}

// Reset deletes all metrics in this vector.
func (m *vecMetrics) Reset() {
	m.inFlightVec.Reset()
	m.successesVec.Reset()
	m.failuresVec.Reset()
	m.errorsVec.Reset()
	m.panicsVec.Reset()
	m.timeoutsVec.Reset()
	m.retriesVec.Reset()

	if m.durationsVec != nil {
		if durationsVec, ok := m.durationsVec.(*prometheus.HistogramVec); ok {
			durationsVec.Reset()
		}
	}
}
