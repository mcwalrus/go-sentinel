package sentinel

// VecObserverConfig embeds ObserverConfig and adds label configuration for Vec metrics.
// When LabelNames is set, mutliple metrics are created for each label value.
type VecObserverConfig struct {
	ObserverConfig

	// LabelNames specifies the names of Prometheus labels to use for Vec metrics for
	// multi-dimensional metrics. All label names must be provided in the same order
	// for all metrics.
	LabelNames []string
}
