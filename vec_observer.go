package sentinel

import (
	"maps"
	"slices"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// VecObserver is an Observer that supports Prometheus labels for multi-dimensional metrics.
// It follows the same pattern as Prometheus's GaugeVec and CounterVec, providing methods
// to create child observers with specific label combinations.
//
// Child observers created from a VecObserver share the same underlying metric vectors.
// This means:
//   - All child observers contribute to the same metric time series, differentiated by labels
//   - Deleting metrics from the VecObserver affects all child observers using those labels
//   - Resetting the VecObserver clears all metrics for all child observers
//   - Child observers maintain their own configuration (timeouts, retries, etc.) but share metrics
//
// Example usage:
//
//	vecObserver := sentinel.NewVecObserver(
//		[]float64{0.1, 0.5, 1, 2, 5},
//		[]string{"service", "environment"},
//	)
//	apiObserver := vecObserver.WithLabelValues("api", "production")
//	dbObserver := vecObserver.WithLabelValues("db", "staging")
type VecObserver struct {
	cfg     config
	metrics vecMetrics
}

// NewVecObserver creates a new VecObserver with label support.
// When LabelNames is set in the VecObserverConfig, Vec metrics are used instead of direct metrics.
//
// Example usage:
//
//	observer := sentinel.NewVecObserver(
//		[]float64{0.1, 0.5, 1, 2, 5},
//		[]string{"service", "status"},
//		sentinel.WithNamespace("myapp"),
//	)
func NewVecObserver(durationBuckets []float64, labelNames []string, opts ...ObserverOption) *VecObserver {
	cfg := setupConfig(durationBuckets, opts...)
	return &VecObserver{
		cfg:     cfg,
		metrics: *newVecMetrics(cfg, labelNames),
	}
}

// Register registers all VecObserver metrics with the provided Prometheus registry.
func (vo *VecObserver) Register(registry prometheus.Registerer) error {
	return vo.metrics.Register(registry)
}

// MustRegister registers all VecObserver metrics with the provided Prometheus registry.
func (vo *VecObserver) MustRegister(registry prometheus.Registerer) {
	vo.metrics.MustRegister(registry)
}

// Describe implements the [prometheus.Collector] interface by describing metrics.
// This can be useful to register the Observer with the default Prometheus registry.
func (vo *VecObserver) Describe(ch chan<- *prometheus.Desc) {
	vo.metrics.Describe(ch)
}

// Collect implements the [prometheus.Collector] interface by collecting metrics.
// This can be useful to register the Observer with the default Prometheus registry.
func (vo *VecObserver) Collect(ch chan<- prometheus.Metric) {
	vo.metrics.Collect(ch)
}

// With returns a new Observer with the given labels. The observer will share the same
// underlying metrics as the original, but specifi
// An error will be returned if the labels do not match initially configured label names.
func (vo *VecObserver) With(labels prometheus.Labels) (*Observer, error) {
	m, err := vo.metrics.with(labels)
	if err != nil {
		return nil, err
	}
	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		metrics:       m,
		labelValues:   slices.Collect(maps.Values(labels)),
		recoverPanics: true,
	}, nil
}

// WithLabels returns a new Observer with the given label values.
// An error will be returned if the label values do not match initially configured label names.
func (vo *VecObserver) WithLabels(labelValues ...string) (*Observer, error) {
	m, err := vo.metrics.withLabels(labelValues...)
	if err != nil {
		return nil, err
	}
	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		metrics:       m,
		labelValues:   labelValues,
		recoverPanics: true,
	}, nil
}

// CurryWithLabels returns a VecObserver curried with the provided labels. Currying allows
// you to pre-set some labels, creating a new VecObserver that requires fewer labels
// for subsequent operations.
// An error will be returned if the labels do not match initially configured label names.
func (vo *VecObserver) CurryWith(labels prometheus.Labels) (*VecObserver, error) {
	curriedMetrics, err := vo.metrics.curryWith(labels)
	if err != nil {
		return nil, err
	}
	return &VecObserver{
		cfg:     vo.cfg,
		metrics: curriedMetrics,
	}, nil
}

// Reset deletes all metrics in this VecObserver.
//
// This method affects all child observers created from this VecObserver. After reset,
// all child observers will continue to work, but all previous metric values will be
// cleared.
//
// This method follows the same pattern as Prometheus's GaugeVec.Reset method.
func (vo *VecObserver) Reset() {
	vo.metrics.reset()
}
