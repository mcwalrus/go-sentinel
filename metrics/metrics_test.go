package main

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCounterVecWithNoLabels(t *testing.T) {
	// Create a CounterVec with no label names
	counterVec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_counter",
			Help: "A test counter",
		},
		[]string{}, // No labels
	)

	// Get the metric by passing no label values
	counter := counterVec.WithLabelValues()
	require.NotNil(t, counter)

	// Use the counter
	counter.Inc()
	counter.Add(5)

	// Alternative: using With() method
	counter2 := counterVec.With(prometheus.Labels{})
	require.NotNil(t, counter2)
	counter2.Inc()
}

func TestGaugeVecWithNoLabels(t *testing.T) {
	// Create a GaugeVec with no label names
	gaugeVec := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "test_gauge",
			Help: "A test gauge",
		},
		[]string{}, // No labels
	)

	// Get the metric by passing no label values
	gauge := gaugeVec.WithLabelValues()
	require.NotNil(t, gauge)

	// Use the gauge
	gauge.Set(42)
	gauge.Inc()
	gauge.Dec()
	gauge.Add(10)
	gauge.Sub(5)

	// Alternative: using With() method
	gauge2 := gaugeVec.With(prometheus.Labels{})
	require.NotNil(t, gauge2)
	gauge2.Set(100)
}

func TestHistogramVecWithNoLabels(t *testing.T) {
	// Create a HistogramVec with no label names
	histogramVec := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "test_histogram",
			Help:    "A test histogram",
			Buckets: prometheus.DefBuckets,
		},
		[]string{}, // No labels
	)

	// Get the metric by passing no label values
	histogram := histogramVec.WithLabelValues()
	require.NotNil(t, histogram)

	// Use the histogram
	histogram.Observe(0.5)
	histogram.Observe(1.2)
	histogram.Observe(3.7)

	// Alternative: using With() method
	histogram2 := histogramVec.With(prometheus.Labels{})
	require.NotNil(t, histogram2)
	histogram2.Observe(2.5)
}

func TestSummaryVecWithNoLabels(t *testing.T) {
	// Create a SummaryVec with no label names
	summaryVec := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "test_summary",
			Help:       "A test summary",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{}, // No labels
	)

	// Get the metric by passing no label values
	summary := summaryVec.WithLabelValues()
	require.NotNil(t, summary)

	// Use the summary
	summary.Observe(0.5)
	summary.Observe(1.2)
	summary.Observe(3.7)

	// Alternative: using With() method
	summary2 := summaryVec.With(prometheus.Labels{})
	require.NotNil(t, summary2)
	summary2.Observe(2.5)
}

func TestVecWithNoLabels_Registration(t *testing.T) {
	// Test that we can register and collect metrics from Vecs with no labels
	registry := prometheus.NewRegistry()

	counterVec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "registered_counter",
			Help: "A registered counter",
		},
		[]string{},
	)

	err := registry.Register(counterVec)
	require.NoError(t, err)

	// Use the counter
	counter := counterVec.WithLabelValues()
	counter.Add(42)

	// Gather metrics to verify
	metrics, err := registry.Gather()
	require.NoError(t, err)
	assert.Len(t, metrics, 1)
	assert.Equal(t, "registered_counter", *metrics[0].Name)
	assert.Len(t, metrics[0].Metric, 1)
	assert.Equal(t, float64(42), *metrics[0].Metric[0].Counter.Value)
}

func TestVecWithNoLabels_CurryingNotApplicable(t *testing.T) {
	// When there are no labels, currying doesn't make sense,
	// but we can verify the behavior
	counterVec := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "curry_test_counter",
			Help: "A test counter for currying",
		},
		[]string{},
	)

	// MustCurryWith with empty labels should work
	curriedVec := counterVec.MustCurryWith(prometheus.Labels{})
	require.NotNil(t, curriedVec)

	// Should still be able to get the metric
	counter := curriedVec.WithLabelValues()
	require.NotNil(t, counter)
	counter.Inc()
}
