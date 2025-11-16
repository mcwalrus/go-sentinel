package sentinel

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// VecObserver is an Observer that supports Prometheus labels for multi-dimensional metrics.
// It embeds Observer and adds a Fork method for creating child observers with merged labels.
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

func (vo *VecObserver) Describe(ch chan<- *prometheus.Desc) {
	vo.metrics.Describe(ch)
}

func (vo *VecObserver) Collect(ch chan<- prometheus.Metric) {
	vo.metrics.Collect(ch)
}

func (vo *VecObserver) Fork(labelValues ...string) *Observer {
	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		metrics:       vo.metrics.withLabels(labelValues...),
		recoverPanics: true,
	}
}

// Fork creates a new VecObserver with merged labels, sharing the same underlying Vec metrics.
// The provided labels are merged with the observer's static labels, with fork labels taking precedence.
//
// Example usage:
//
//	baseObserver := sentinel.NewVecObserver(
//		nil,
//		sentinel.VecObserverConfig{
//			ObserverConfig: sentinel.ObserverConfig{},
//			LabelNames:     []string{"service", "environment"},
//		},
//	)
//	serviceObserver := baseObserver.Fork("api", "production")
//	// Metrics will have: service="api", environment="production"
func (vo *VecObserver) ForkWith(labels prometheus.Labels) *Observer {
	return &Observer{
		m:             &sync.RWMutex{},
		cfg:           vo.cfg,
		metrics:       vo.metrics.with(labels),
		recoverPanics: true,
	}
}

// CurryWith creates a new VecObserver with merged labels, sharing the same underlying Vec metrics.
func (vo *VecObserver) CurryWith(labels prometheus.Labels) *VecObserver {
	return &VecObserver{
		cfg:     vo.cfg,
		metrics: vo.metrics.CurryWith(labels),
	}
}
