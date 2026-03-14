package sentinel

import (
	"maps"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/sourcegraph/conc/pool"
)

// VecObserver is a labeled Observer that supports Prometheus label dimensions.
// Use [VecObserver.With] or [VecObserver.WithLabels] to get an Observer for a
// specific label set. Create with [NewVecObserver].
//
// Example usage:
//
//	vecObserver := sentinel.NewVecObserver(
//		[]string{"service", "environment"},
//		sentinel.WithDurationMetrics([]float64{0.1, 0.5, 1, 2, 5}),
//	)
//	apiObserver, _ := vecObserver.WithLabels("api", "production")
//	dbObserver, _ := vecObserver.WithLabels("db", "staging")
type VecObserver struct {
	cfg     config
	metrics vecMetrics
}

// NewVecObserver returns a new VecObserver with the given label names and options.
// labelNames defines the Prometheus label dimensions; each call to [VecObserver.With]
// or [VecObserver.WithLabels] must supply values for every name in this slice.
// opts configure which metrics are recorded, namespace, subsystem, and other settings.
//
// Example usage:
//
//	observer := sentinel.NewVecObserver(
//		[]string{"service", "status"},
//		sentinel.WithDurationMetrics([]float64{0.1, 0.5, 1, 2, 5}),
//		sentinel.WithNamespace("myapp"),
//	)
func NewVecObserver(labelNames []string, opts ...ObserverOption) *VecObserver {
	cfg := setupConfig(opts...)
	return &VecObserver{
		cfg:     cfg,
		metrics: *newVecMetrics(cfg, labelNames),
	}
}

// Register registers all VecObserver metrics with the provided Prometheus registry.
// This method returns an error if any metric registration fails. Use [VecObserver.MustRegister]
// if you want the program to panic on registration conflicts instead of handling errors.
//
// Example usage:
//
//	registry := prometheus.NewRegistry()
//	if err := vecObserver.Register(registry); err != nil {
//		log.Fatalf("Failed to register metrics: %v", err)
//	}
func (vo *VecObserver) Register(registry prometheus.Registerer) error {
	return vo.metrics.Register(registry)
}

// MustRegister registers all VecObserver metrics with the provided Prometheus registry.
// This method panics if any metric registration failures occur. Use [VecObserver.Register]
// if you prefer to handle registration errors gracefully instead of panicking.
//
// Example usage:
//
//	registry := prometheus.NewRegistry()
//	vecObserver.MustRegister(registry) // Will panic if registration fails
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
// underlying metrics as the original, but will record metrics with the specified label values.
// An error will be returned if the labels do not match initially configured label names.
//
// Example usage:
//
//	vecObserver := sentinel.NewVecObserver(
//		[]string{"service", "environment"},
//		sentinel.WithDurationMetrics([]float64{0.1, 0.5, 1, 2, 5}),
//	)
//	observer, err := vecObserver.With(prometheus.Labels{
//		"service":     "api",
//		"environment": "production",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
func (vo *VecObserver) With(labels prometheus.Labels) (*Observer, error) {
	m, err := vo.metrics.with(labels)
	if err != nil {
		return nil, err
	}
	return &Observer{
		cfg:           vo.cfg,
		metrics:       m,
		pool:          pool.New(),
		labelValues:   slices.Collect(maps.Values(labels)),
		recoverPanics: true,
	}, nil
}

// WithLabels returns a new Observer with the given label values in the same order as
// the label names provided to [NewVecObserver]. An error will be returned if the number
// of label values does not match the number of label names.
//
// Example usage:
//
//	vecObserver := sentinel.NewVecObserver(
//		[]string{"service", "environment"},
//		sentinel.WithDurationMetrics([]float64{0.1, 0.5, 1, 2, 5}),
//	)
//	observer, err := vecObserver.WithLabels("api", "production")
//	if err != nil {
//		log.Fatal(err)
//	}
func (vo *VecObserver) WithLabels(labelValues ...string) (*Observer, error) {
	m, err := vo.metrics.withLabels(labelValues...)
	if err != nil {
		return nil, err
	}
	return &Observer{
		cfg:           vo.cfg,
		metrics:       m,
		pool:          pool.New(),
		labelValues:   labelValues,
		recoverPanics: true,
	}, nil
}

// CurryWith returns a VecObserver curried with the provided labels. Currying allows
// you to pre-set some labels, creating a new VecObserver that requires fewer labels
// for subsequent operations. This is useful when you have a common label value (like
// "environment") that you want to apply to multiple observers.
//
// An error will be returned if the labels do not match initially configured label names.
//
// Example usage:
//
//	vecObserver := sentinel.NewVecObserver(
//		[]string{"service", "environment"},
//		sentinel.WithDurationMetrics([]float64{0.1, 0.5, 1, 2, 5}),
//	)
//	// Curry with environment="production", now only service label is needed
//	prodObserver, err := vecObserver.CurryWith(prometheus.Labels{"environment": "production"})
//	if err != nil {
//		log.Fatal(err)
//	}
//	// Create observers with only the service label
//	apiObserver, _ := prodObserver.WithLabels("api")
//	dbObserver, _ := prodObserver.WithLabels("db")
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
// This method affects all child observers created from this VecObserver. After reset:
//
//   - All metric values tracked by the VecObserver are cleared
//   - Existing child observers created before Reset() can still be called without panicking,
//     but their metrics will not be tracked by the VecObserver (they record to old metric
//     instances that were removed from the Vec's internal map)
//   - To continue tracking metrics after Reset(), create new child observers using
//     [VecObserver.WithLabels] or [VecObserver.With]
//
// This method follows the same pattern as Prometheus's GaugeVec.Reset method.
func (vo *VecObserver) Reset() {
	vo.metrics.reset()
}
