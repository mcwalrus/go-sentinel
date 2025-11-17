package sentinel

import (
	"fmt"
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
	m             *sync.RWMutex
	cfg           config
	runner        ObserverConfig
	controls      ObserverControls
	metrics       vecMetrics
	recoverPanics bool
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
		m:             &sync.RWMutex{},
		cfg:           cfg,
		metrics:       *newVecMetrics(cfg, labelNames),
		recoverPanics: true,
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

func (vo *VecObserver) Describe(ch chan<- *prometheus.Desc) {
	vo.metrics.Describe(ch)
}

func (vo *VecObserver) Collect(ch chan<- prometheus.Metric) {
	vo.metrics.Collect(ch)
}

// UseConfig configures the observer for how to handle Run methods.
// This sets the ObserverConfig that will be used for all subsequent Run, RunFunc calls.
// See [ObserverConfig] for more information on available configuration options.
//
// Example usage:
//
//	observer.UseConfig(sentinel.ObserverConfig{
//		Timeout:    10 * time.Second,
//		MaxRetries: 3,
//		RetryStrategy: retry.Exponential(100 * time.Millisecond),
//	})
func (vo *VecObserver) UseConfig(config ObserverConfig) {
	vo.m.Lock()
	vo.runner = config
	vo.controls = config.Controls
	vo.m.Unlock()
}

// DisablePanicRecovery sets whether panic recovery should be disabled for the observer.
// Recovery is enabled by default, meaning panics are caught and converted to errors.
func (vo *VecObserver) DisablePanicRecovery(disable bool) {
	vo.m.Lock()
	vo.recoverPanics = !disable
	vo.m.Unlock()
}

func (vo *VecObserver) With(labels prometheus.Labels) (*Observer, error) {
	m, err := vo.metrics.with(labels)
	if err != nil {
		return nil, err
	}
	vo.m.RLock()
	defer vo.m.RUnlock()

	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		runner:        vo.runner,
		controls:      vo.controls,
		metrics:       m,
		labelValues:   slices.Collect(maps.Values(labels)),
		recoverPanics: vo.recoverPanics,
		parent:        vo,
	}, nil
}

func (vo *VecObserver) WithLabels(labelValues ...string) (*Observer, error) {
	m, err := vo.metrics.withLabelValues(labelValues...)
	if err != nil {
		return nil, err
	}
	vo.m.RLock()
	defer vo.m.RUnlock()

	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		metrics:       m,
		runner:        vo.runner,
		controls:      vo.controls,
		labelValues:   labelValues,
		recoverPanics: vo.recoverPanics,
		parent:        vo,
	}, nil
}

// With returns an Observer with the given labels. This method follows the same pattern
// as Prometheus's GaugeVec.With method.
//
// The returned Observer shares the same underlying metric vectors as the VecObserver.
// All metrics recorded by the returned Observer will have the specified labels.
//
// Panics if the number of labels does not match the number of label names.
func (vo *VecObserver) MustWith(labels prometheus.Labels) *Observer {
	vo.m.RLock()
	defer vo.m.RUnlock()

	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		metrics:       vo.metrics.withMust(labels),
		labelValues:   slices.Collect(maps.Values(labels)),
		recoverPanics: vo.recoverPanics,
		parent:        vo,
	}
}

// WithLabels returns an Observer with the given label values. This method follows
// the same pattern as Prometheus's GaugeVec.WithLabelValues method.
//
// The returned Observer shares the same underlying metric vectors as the VecObserver.
// All metrics recorded by the returned Observer will have the specified label values.
//
// Panics if the number of label values does not match the number of label names.
func (vo *VecObserver) MustWithLabels(labelValues ...string) *Observer {
	vo.m.RLock()
	defer vo.m.RUnlock()

	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		metrics:       vo.metrics.withLabelsMust(labelValues...),
		labelValues:   labelValues,
		recoverPanics: vo.recoverPanics,
		parent:        vo,
	}
}

// CurryWith returns a VecObserver curried with the provided labels. This method follows
// the same pattern as Prometheus's GaugeVec.CurryWith method.
//
// Currying allows you to pre-set some labels, creating a new VecObserver that requires
// fewer labels for subsequent operations. The number of provided labels must be less
// than the total number of label names.
//
// The returned VecObserver shares the same underlying metric vectors as the original.
func (vo *VecObserver) CurryWith(labels prometheus.Labels) (*VecObserver, error) {
	curriedMetrics, err := vo.metrics.CurryWith(labels)
	if err != nil {
		return nil, err
	}

	vo.m.RLock()
	defer vo.m.RUnlock()

	return &VecObserver{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		runner:        vo.runner,
		controls:      vo.controls,
		recoverPanics: vo.recoverPanics,
		metrics:       curriedMetrics,
	}, nil
}

// MustCurryWith works as CurryWith but panics where CurryWith would have returned an error.
// This method follows the same pattern as Prometheus's GaugeVec.MustCurryWith method.
func (vo *VecObserver) MustCurryWith(labels prometheus.Labels) *VecObserver {
	curried, err := vo.CurryWith(labels)
	if err != nil {
		panic(err)
	}
	return curried
}

func (vo *VecObserver) Revoke(ob *Observer) error {
	if ob.parent != vo {
		return fmt.Errorf("observer %v is not a child of this VecObserver", ob.labelValues)
	}
	if vo.metrics.DeleteLabelValues(ob.labelValues...) {
		return nil
	}
	return fmt.Errorf("failed to unregister observer")
}

// Reset deletes all metrics in this VecObserver.
//
// This method affects all child observers created from this VecObserver. After reset,
// all child observers will continue to work, but all previous metric values will be
// cleared.
//
// This method follows the same pattern as Prometheus's GaugeVec.Reset method.
func (vo *VecObserver) Reset() {
	vo.metrics.Reset()
}
