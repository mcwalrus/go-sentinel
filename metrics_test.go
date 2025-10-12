package sentinel

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testConfig(t *testing.T) config {
	t.Helper()
	return config{
		namespace:   "test",
		subsystem:   "metrics",
		description: "test operations",
		buckets:     []float64{0.01, 0.1, 1, 10, 100},
	}
}

func TestMetricsMustRegister(t *testing.T) {
	t.Parallel()

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
		"test_metrics_success_total",
		"test_metrics_errors_total",
		"test_metrics_timeouts_total",
		"test_metrics_panics_total",
		"test_metrics_durations_seconds",
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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	cfg := testConfig(t)
	m := newMetrics(cfg)
	registry := prometheus.NewRegistry()
	m.MustRegister(registry)

	// Increment metrics
	m.Successes.Inc()
	m.Errors.Inc()
	m.Errors.Inc()
	m.Timeouts.Inc()
	m.Panics.Inc()
	m.Retries.Inc()
	m.InFlight.Inc()
	m.InFlight.Inc()
	m.InFlight.Dec()

	// Observe metrics
	m.Durations.Observe(0.05)
	m.Durations.Observe(0.5)
	m.Durations.Observe(5.0)

	// Verify metrics
	if got := testutil.ToFloat64(m.Successes); got != 1 {
		t.Errorf("Expected Successes=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.Errors); got != 2 {
		t.Errorf("Expected Errors=2, got %f", got)
	}
	if got := testutil.ToFloat64(m.Timeouts); got != 1 {
		t.Errorf("Expected Timeouts=1, got %f", got)
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
		if *family.Name == "test_metrics_durations_seconds" {
			if len(family.Metric) > 0 && family.Metric[0].Histogram != nil {
				histogramSampleCount = *family.Metric[0].Histogram.SampleCount
				break
			}
		}
	}

	if histogramSampleCount != 3 {
		t.Errorf("Expected Durations count=3, got %d", histogramSampleCount)
	}
}

func TestMetricLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config
		expected map[string]string
	}{
		{
			name: "no namespace or subsystem",
			cfg: config{
				namespace:   "",
				subsystem:   "",
				description: "tasks",
			},
			expected: map[string]string{
				"in_flight":      "in_flight",
				"success_total":  "success_total",
				"failures_total": "failures_total",
				"errors_total":   "errors_total",
				"panics_total":   "panics_total",
				"timeouts_total": "timeouts_total",
				"retries_total":  "retries_total",
			},
		},
		{
			name: "no namespace or subsystem with additional metrics",
			cfg: config{
				namespace:   "",
				subsystem:   "",
				description: "tasks",
				buckets:     []float64{0.1, 1},
			},
			expected: map[string]string{
				"in_flight":         "in_flight",
				"success_total":     "success_total",
				"failures_total":    "failures_total",
				"errors_total":      "errors_total",
				"panics_total":      "panics_total",
				"timeouts_total":    "timeouts_total",
				"retries_total":     "retries_total",
				"durations_seconds": "durations_seconds",
			},
		},
		{
			name: "with namespace and subsystem",
			cfg: config{
				namespace:   "myapp",
				subsystem:   "workers",
				description: "background tasks",
			},
			expected: map[string]string{
				"in_flight":      "myapp_workers_in_flight",
				"success_total":  "myapp_workers_success_total",
				"failures_total": "myapp_workers_failures_total",
				"errors_total":   "myapp_workers_errors_total",
				"panics_total":   "myapp_workers_panics_total",
				"timeouts_total": "myapp_workers_timeouts_total",
				"retries_total":  "myapp_workers_retries_total",
			},
		},
		{
			name: "with namespace and subsystem with additional metrics",
			cfg: config{
				namespace:   "myapp",
				subsystem:   "workers",
				description: "background tasks",
				buckets:     []float64{0.1, 1},
			},
			expected: map[string]string{
				"in_flight":         "myapp_workers_in_flight",
				"success_total":     "myapp_workers_success_total",
				"failures_total":    "myapp_workers_failures_total",
				"errors_total":      "myapp_workers_errors_total",
				"panics_total":      "myapp_workers_panics_total",
				"timeouts_total":    "myapp_workers_timeouts_total",
				"retries_total":     "myapp_workers_retries_total",
				"durations_seconds": "myapp_workers_durations_seconds",
			},
		},
		{
			name: "subsystem only",
			cfg: config{
				namespace:   "",
				subsystem:   "api",
				description: "API calls",
			},
			expected: map[string]string{
				"in_flight":      "api_in_flight",
				"success_total":  "api_success_total",
				"failures_total": "api_failures_total",
				"errors_total":   "api_errors_total",
				"panics_total":   "api_panics_total",
				"timeouts_total": "api_timeouts_total",
				"retries_total":  "api_retries_total",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			t.Logf("testing: %s", tt.name)

			m := newMetrics(tt.cfg)
			registry := prometheus.NewRegistry()
			m.MustRegister(registry)

			families, err := registry.Gather()
			if err != nil {
				t.Fatalf("Failed to gather metrics: %v", err)
			}

			if len(families) != len(tt.expected) {
				t.Errorf(
					"Expected %d metrics, got %d",
					len(tt.expected), len(families),
				)
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
	t.Parallel()

	t.Run("with additional metrics", func(t *testing.T) {
		cfg := config{
			namespace:   "",
			subsystem:   "",
			description: "test operations",
			buckets:     []float64{0.1, 1},
		}

		m := newMetrics(cfg)
		registry := prometheus.NewRegistry()
		m.MustRegister(registry)

		families, err := registry.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		expectedHelpTexts := map[string]string{
			"in_flight":         "Number of observed test operations in flight",
			"success_total":     "Number of successes from observed test operations",
			"failures_total":    "Number of failures from observed test operations excluding retry attempts",
			"errors_total":      "Number of errors from observed test operations including retries and panics",
			"panics_total":      "Number of panic occurrences from observed test operations",
			"durations_seconds": "Histogram of the observed durations of test operations",
			"timeouts_total":    "Number of timeout errors from observed test operations",
			"retries_total":     "Number of retry attempts from observed test operations",
		}

		t.Log("Checking help text")
		if len(families) != len(expectedHelpTexts) {
			t.Errorf(
				"Expected %d metrics, got %d",
				len(expectedHelpTexts), len(families),
			)
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
	})
}

func TestMetricHelpText1(t *testing.T) {
	t.Parallel()

	t.Run("default description should be used", func(t *testing.T) {

		t.Log("default description should be used")
		observer := NewObserver(
			[]float64{0.1, 1},
			WithDescription(""), // no description
		)

		m := observer.metrics
		registry := prometheus.NewRegistry()
		m.MustRegister(registry)

		families, err := registry.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		expectedHelpTexts := map[string]string{
			"in_flight":         "Number of observed tasks in flight",
			"success_total":     "Number of successes from observed tasks",
			"failures_total":    "Number of failures from observed tasks excluding retry attempts",
			"errors_total":      "Number of errors from observed tasks including retries and panics",
			"panics_total":      "Number of panic occurrences from observed tasks",
			"durations_seconds": "Histogram of the observed durations of tasks",
			"timeouts_total":    "Number of timeout errors from observed tasks",
			"retries_total":     "Number of retry attempts from observed tasks",
		}

		t.Log("Checking help text")
		if len(families) != len(expectedHelpTexts) {
			t.Errorf(
				"Expected %d metrics, got %d",
				len(expectedHelpTexts), len(families),
			)
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
	})
}
