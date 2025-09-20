package sentinel

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testConfig(t *testing.T) ObserverConfig {
	t.Helper()
	return ObserverConfig{
		Namespace:       "test",
		Subsystem:       "metrics",
		Description:     "test operations",
		BucketDurations: []float64{0.01, 0.1, 1, 10, 100},
	}
}

func TestMetricsMustRegister(t *testing.T) {
	cfg := testConfig(t)
	m := newMetrics(cfg)
	registry := prometheus.NewRegistry()

	t.Log("Registering metrics")
	m.MustRegister(registry)

	t.Log("Gathering metrics")
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	expectedMetrics := []string{
		"test_metrics_in_flight",
		"test_metrics_successes_total",
		"test_metrics_errors_total",
		"test_metrics_timeouts_total",
		"test_metrics_panics_total",
		"test_metrics_observed_duration_seconds",
		"test_metrics_retries_total",
	}

	foundMetrics := make(map[string]bool)
	for _, family := range families {
		foundMetrics[*family.Name] = true
	}

	t.Logf("Checking exposed metrics")
	for _, expected := range expectedMetrics {
		if !foundMetrics[expected] {
			t.Errorf("Expected metric %s not found in registry", expected)
		}
	}
}

func TestMetricsMustRegisterPanic(t *testing.T) {
	cfg := testConfig(t)
	m := newMetrics(cfg)
	registry := prometheus.NewRegistry()

	// First registration should succeed
	m.MustRegister(registry)

	// Second registration should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustRegister to panic on duplicate registration")
		}
	}()

	m.MustRegister(registry)
}

func TestMetricsRegister(t *testing.T) {
	cfg := testConfig(t)
	m := newMetrics(cfg)
	registry := prometheus.NewRegistry()

	// Register metrics
	err := m.Register(registry)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}

	// Expect error on duplicate registration
	err = m.Register(registry)
	if err == nil {
		t.Error("Expected Register to return error on duplicate registration")
	}

	// Validate error is AlreadyRegisteredError
	var alreadyRegisteredError prometheus.AlreadyRegisteredError
	if !errors.As(err, &alreadyRegisteredError) {
		t.Error("Expected error to contain AlreadyRegisteredError")
	}
}

func TestMetricUpdates(t *testing.T) {
	cfg := testConfig(t)
	m := newMetrics(cfg)
	registry := prometheus.NewRegistry()
	m.MustRegister(registry)

	// Increment metrics
	m.Successes.Inc()
	m.Errors.Inc()
	m.Errors.Inc()
	m.TimeoutErrors.Inc()
	m.Panics.Inc()
	m.Retries.Inc()
	m.InFlight.Inc()
	m.InFlight.Inc()
	m.InFlight.Dec()

	// Observe metrics
	m.ObservedRuntimes.Observe(0.05)
	m.ObservedRuntimes.Observe(0.5)
	m.ObservedRuntimes.Observe(5.0)

	// Verify metrics
	if got := testutil.ToFloat64(m.Successes); got != 1 {
		t.Errorf("Expected Successes=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.Errors); got != 2 {
		t.Errorf("Expected Errors=2, got %f", got)
	}
	if got := testutil.ToFloat64(m.TimeoutErrors); got != 1 {
		t.Errorf("Expected TimeoutErrors=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.Panics); got != 1 {
		t.Errorf("Expected Panics=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.Retries); got != 1 {
		t.Errorf("Expected Retries=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.InFlight); got != 1 {
		t.Errorf("Expected InFlight=1, got %f", got)
	}
	// For histograms, we need to get the sample count differently
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics for histogram check: %v", err)
	}

	var histogramSampleCount uint64
	for _, family := range families {
		if *family.Name == "test_metrics_observed_duration_seconds" {
			if len(family.Metric) > 0 && family.Metric[0].Histogram != nil {
				histogramSampleCount = *family.Metric[0].Histogram.SampleCount
				break
			}
		}
	}

	if histogramSampleCount != 3 {
		t.Errorf("Expected ObservedRuntimes count=3, got %d", histogramSampleCount)
	}
}

func TestMetricLabels(t *testing.T) {
	tests := []struct {
		name     string
		cfg      ObserverConfig
		expected map[string]string
	}{
		{
			name: "no namespace or subsystem",
			cfg: ObserverConfig{
				Namespace:       "",
				Subsystem:       "",
				Description:     "tasks",
				BucketDurations: []float64{0.1, 1},
			},
			expected: map[string]string{
				"in_flight":                 "in_flight",
				"successes_total":           "successes_total",
				"errors_total":              "errors_total",
				"timeouts_total":            "timeouts_total",
				"panics_total":              "panics_total",
				"observed_duration_seconds": "observed_duration_seconds",
				"retries_total":             "retries_total",
			},
		},
		{
			name: "with namespace and subsystem",
			cfg: ObserverConfig{
				Namespace:       "myapp",
				Subsystem:       "workers",
				Description:     "background tasks",
				BucketDurations: []float64{0.1, 1},
			},
			expected: map[string]string{
				"in_flight":                 "myapp_workers_in_flight",
				"successes_total":           "myapp_workers_successes_total",
				"errors_total":              "myapp_workers_errors_total",
				"timeouts_total":            "myapp_workers_timeouts_total",
				"panics_total":              "myapp_workers_panics_total",
				"observed_duration_seconds": "myapp_workers_observed_duration_seconds",
				"retries_total":             "myapp_workers_retries_total",
			},
		},
		{
			name: "subsystem only",
			cfg: ObserverConfig{
				Namespace:       "",
				Subsystem:       "api",
				Description:     "API calls",
				BucketDurations: []float64{0.1, 1},
			},
			expected: map[string]string{
				"in_flight":                 "api_in_flight",
				"successes_total":           "api_successes_total",
				"errors_total":              "api_errors_total",
				"timeouts_total":            "api_timeouts_total",
				"panics_total":              "api_panics_total",
				"observed_duration_seconds": "api_observed_duration_seconds",
				"retries_total":             "api_retries_total",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMetrics(tt.cfg)
			registry := prometheus.NewRegistry()
			m.MustRegister(registry)

			families, err := registry.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			foundMetrics := make(map[string]bool)
			for _, family := range families {
				foundMetrics[*family.Name] = true
			}

			for _, expectedName := range tt.expected {
				if !foundMetrics[expectedName] {
					t.Errorf("Expected metric %s not found", expectedName)
				}
			}
		})
	}
}

func TestMetricHelpText(t *testing.T) {
	cfg := ObserverConfig{
		Namespace:       "",
		Subsystem:       "",
		Description:     "test operations",
		BucketDurations: []float64{0.1, 1},
	}

	m := newMetrics(cfg)
	registry := prometheus.NewRegistry()
	m.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	expectedHelpTexts := map[string]string{
		"in_flight":                 "Number of observed test operations in flight",
		"successes_total":           "Number of successes from observed test operations",
		"errors_total":              "Number of errors from observed test operations",
		"timeouts_total":            "Number of timeout errors from observed test operations",
		"panics_total":              "Number of panic occurances from observed test operations",
		"observed_duration_seconds": "Histogram of the observed durations of test operations",
		"retries_total":             "Number of retry attempts from observed test operations",
	}

	for _, family := range families {
		expectedHelp, exists := expectedHelpTexts[*family.Name]
		if !exists {
			continue
		}
		if *family.Help != expectedHelp {
			t.Errorf("Metric %s: expected help text %q, got %q",
				*family.Name, expectedHelp, *family.Help)
		}
	}
}
