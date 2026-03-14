package sentinel

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testConfig(t *testing.T) config {
	t.Helper()
	return config{
		namespace:       "test",
		subsystem:       "metrics",
		description:     "test operations",
		DurationBuckets: []float64{0.01, 0.1, 1, 10, 100},
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
	m.successes.Inc()
	m.errors.Inc()
	m.errors.Inc()

	m.timeouts.Inc()
	m.panics.Inc()
	m.retries.Inc()
	m.inFlight.Inc()

	// Observe metrics
	m.durations.Observe(0.05)
	m.durations.Observe(0.5)
	m.durations.Observe(5.0)

	// Verify metrics
	if got := testutil.ToFloat64(m.successes); got != 1 {
		t.Errorf("Expected Successes=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.errors); got != 2 {
		t.Errorf("Expected Errors=2, got %f", got)
	}
	if got := testutil.ToFloat64(m.timeouts); got != 1 {
		t.Errorf("Expected Timeouts=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.panics); got != 1 {
		t.Errorf("Expected Panics=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.retries); got != 1 {
		t.Errorf("Expected Retries=1, got %f", got)
	}
	if got := testutil.ToFloat64(m.inFlight); got != 1 {
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
				"pending_total":  "pending_total",
			},
		},
		{
			name: "no namespace or subsystem with additional metrics",
			cfg: config{
				namespace:       "",
				subsystem:       "",
				description:     "tasks",
				DurationBuckets: []float64{0.1, 1},
			},
			expected: map[string]string{
				"in_flight":         "in_flight",
				"success_total":     "success_total",
				"failures_total":    "failures_total",
				"errors_total":      "errors_total",
				"panics_total":      "panics_total",
				"timeouts_total":    "timeouts_total",
				"retries_total":     "retries_total",
				"pending_total":     "pending_total",
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
				"pending_total":  "myapp_workers_pending_total",
			},
		},
		{
			name: "with namespace and subsystem with additional metrics",
			cfg: config{
				namespace:       "myapp",
				subsystem:       "workers",
				description:     "background tasks",
				DurationBuckets: []float64{0.1, 1},
			},
			expected: map[string]string{
				"in_flight":         "myapp_workers_in_flight",
				"success_total":     "myapp_workers_success_total",
				"failures_total":    "myapp_workers_failures_total",
				"errors_total":      "myapp_workers_errors_total",
				"panics_total":      "myapp_workers_panics_total",
				"timeouts_total":    "myapp_workers_timeouts_total",
				"retries_total":     "myapp_workers_retries_total",
				"pending_total":     "myapp_workers_pending_total",
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
				"pending_total":  "api_pending_total",
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
			namespace:       "",
			subsystem:       "",
			description:     "test operations",
			DurationBuckets: []float64{0.1, 1},
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
			"pending_total":     "Number of observed test operations pending concurrency limiter acquisition",
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
			WithDurationMetrics([]float64{0.1, 1}),
			WithInFlightMetrics(),
			WithSuccessMetrics(),
			WithErrorMetrics(),
			WithTimeoutMetrics(),
			WithPanicMetrics(),
			WithRetryMetrics(),
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

func TestWithInFlightMetrics(t *testing.T) {
	t.Parallel()

	cfg := config{}
	opt := WithInFlightMetrics()
	opt(&cfg)

	if !cfg.enableInFlight {
		t.Error("WithInFlightMetrics() should set enableInFlight = true")
	}
	if cfg.enableSuccess {
		t.Error("WithInFlightMetrics() should not affect enableSuccess")
	}
}

func TestWithSuccessMetrics(t *testing.T) {
	t.Parallel()

	cfg := config{}
	opt := WithSuccessMetrics()
	opt(&cfg)

	if !cfg.enableSuccess {
		t.Error("WithSuccessMetrics() should set enableSuccess = true")
	}
	if cfg.enableInFlight {
		t.Error("WithSuccessMetrics() should not affect enableInFlight")
	}
}

func TestWithErrorMetrics(t *testing.T) {
	t.Parallel()

	cfg := config{}
	opt := WithErrorMetrics()
	opt(&cfg)

	if !cfg.enableErrors {
		t.Error("WithErrorMetrics() should set enableErrors = true")
	}
	if !cfg.enableFailures {
		t.Error("WithErrorMetrics() should set enableFailures = true")
	}
	if cfg.enableTimeouts {
		t.Error("WithErrorMetrics() should not affect enableTimeouts")
	}
}

func TestWithTimeoutMetrics(t *testing.T) {
	t.Parallel()

	cfg := config{}
	opt := WithTimeoutMetrics()
	opt(&cfg)

	if !cfg.enableTimeouts {
		t.Error("WithTimeoutMetrics() should set enableTimeouts = true")
	}
	if cfg.enableErrors {
		t.Error("WithTimeoutMetrics() should not affect enableErrors")
	}
	if cfg.enableFailures {
		t.Error("WithTimeoutMetrics() should not affect enableFailures")
	}
}

func TestConfigEnableFlagsDefaultToFalse(t *testing.T) {
	t.Parallel()

	cfg := config{}

	if cfg.enableInFlight {
		t.Error("enableInFlight should default to false")
	}
	if cfg.enableSuccess {
		t.Error("enableSuccess should default to false")
	}
	if cfg.enableErrors {
		t.Error("enableErrors should default to false")
	}
	if cfg.enableFailures {
		t.Error("enableFailures should default to false")
	}
	if cfg.enablePanics {
		t.Error("enablePanics should default to false")
	}
	if cfg.enableRetries {
		t.Error("enableRetries should default to false")
	}
	if cfg.enableTimeouts {
		t.Error("enableTimeouts should default to false")
	}
	if cfg.enableDurations {
		t.Error("enableDurations should default to false")
	}
	if cfg.enablePending {
		t.Error("enablePending should default to false")
	}
	if cfg.DurationBuckets != nil {
		t.Error("DurationBuckets should default to nil")
	}
	if cfg.ErrorLabeler != nil {
		t.Error("ErrorLabeler should default to nil")
	}
}

// TestConditionalMetricRegistration verifies that only enabled metrics are registered.

func TestConditionalMetrics_OnlySuccessRegistered(t *testing.T) {
	t.Parallel()

	observer := NewObserver(WithSuccessMetrics())
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(families) != 1 {
		t.Errorf("Expected 1 metric family, got %d", len(families))
	}
	if len(families) > 0 && *families[0].Name != "sentinel_success_total" {
		t.Errorf("Expected sentinel_success_total, got %s", *families[0].Name)
	}
}

func TestConditionalMetrics_AllOptionsEnabled(t *testing.T) {
	t.Parallel()

	observer := NewObserver(
		WithDurationMetrics([]float64{0.1, 1}),
		WithInFlightMetrics(),
		WithSuccessMetrics(),
		WithErrorMetrics(),
		WithTimeoutMetrics(),
		WithQueueMetrics(),
		WithPanicMetrics(),
		WithRetryMetrics(),
	)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	expectedNames := map[string]bool{
		"sentinel_in_flight":         true,
		"sentinel_success_total":     true,
		"sentinel_errors_total":      true,
		"sentinel_failures_total":    true,
		"sentinel_timeouts_total":    true,
		"sentinel_pending_total":     true,
		"sentinel_durations_seconds": true,
		"sentinel_panics_total":      true,
		"sentinel_retries_total":     true,
	}

	for _, family := range families {
		if !expectedNames[*family.Name] {
			t.Errorf("Unexpected metric: %s", *family.Name)
		}
	}
	if len(families) != len(expectedNames) {
		t.Errorf("Expected %d metric families, got %d", len(expectedNames), len(families))
	}
}

func TestConditionalMetrics_NoMetrics_EmptyOutput(t *testing.T) {
	t.Parallel()

	// Create metrics directly with an explicitly empty (non-nil) filter,
	// bypassing the setupConfig fallback to defaultMetricFilter.
	cfg := config{
		namespace:    "test",
		subsystem:    "empty",
		description:  "tasks",
		metricFilter: make(map[string]bool), // non-nil empty: no metrics enabled
	}
	m := newMetrics(cfg)
	registry := prometheus.NewRegistry()
	m.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(families) != 0 {
		t.Errorf("Expected 0 metric families, got %d", len(families))
	}
}

func TestConditionalMetrics_NoMetrics_TasksExecute(t *testing.T) {
	t.Parallel()

	// Tasks should still execute normally even with no metrics enabled.
	observer := NewObserver(WithMetrics())

	executed := false
	err := observer.RunFunc(func(_ context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !executed {
		t.Error("Expected task to execute")
	}
}

func TestConditionalMetrics_TwoObservers_DifferentSubsets_NoConflict(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()

	obs1 := NewObserver(WithSuccessMetrics(), WithNamespace("obs1"))
	obs2 := NewObserver(WithErrorMetrics(), WithNamespace("obs2"))

	if err := obs1.Register(registry); err != nil {
		t.Fatalf("obs1 registration failed: %v", err)
	}
	if err := obs2.Register(registry); err != nil {
		t.Fatalf("obs2 registration failed: %v", err)
	}

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	foundNames := make(map[string]bool)
	for _, f := range families {
		foundNames[*f.Name] = true
	}

	if !foundNames["obs1_success_total"] {
		t.Error("Expected obs1_success_total")
	}
	if !foundNames["obs2_errors_total"] {
		t.Error("Expected obs2_errors_total")
	}
	if !foundNames["obs2_failures_total"] {
		t.Error("Expected obs2_failures_total")
	}
}

func TestWithPanicMetrics(t *testing.T) {
	t.Parallel()

	cfg := config{}
	opt := WithPanicMetrics()
	opt(&cfg)

	if !cfg.enablePanics {
		t.Error("WithPanicMetrics() should set enablePanics = true")
	}
	if cfg.enableRetries {
		t.Error("WithPanicMetrics() should not affect enableRetries")
	}
}

func TestWithRetryMetrics(t *testing.T) {
	t.Parallel()

	cfg := config{}
	opt := WithRetryMetrics()
	opt(&cfg)

	if !cfg.enableRetries {
		t.Error("WithRetryMetrics() should set enableRetries = true")
	}
	if cfg.enablePanics {
		t.Error("WithRetryMetrics() should not affect enablePanics")
	}
}

func TestConditionalMetrics_OnlyPanicsRegistered(t *testing.T) {
	t.Parallel()

	observer := NewObserver(WithPanicMetrics())
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(families) != 1 {
		t.Errorf("Expected 1 metric family, got %d", len(families))
	}
	if len(families) > 0 && *families[0].Name != "sentinel_panics_total" {
		t.Errorf("Expected sentinel_panics_total, got %s", *families[0].Name)
	}
}

func TestConditionalMetrics_OnlyRetriesRegistered(t *testing.T) {
	t.Parallel()

	observer := NewObserver(WithRetryMetrics())
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(families) != 1 {
		t.Errorf("Expected 1 metric family, got %d", len(families))
	}
	if len(families) > 0 && *families[0].Name != "sentinel_retries_total" {
		t.Errorf("Expected sentinel_retries_total, got %s", *families[0].Name)
	}
}

func TestConditionalMetrics_PanicsAndRetriesRegisteredIndependently(t *testing.T) {
	t.Parallel()

	observer := NewObserver(WithPanicMetrics(), WithRetryMetrics())
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	if len(families) != 2 {
		t.Errorf("Expected 2 metric families, got %d", len(families))
	}

	foundNames := make(map[string]bool)
	for _, f := range families {
		foundNames[*f.Name] = true
	}
	if !foundNames["sentinel_panics_total"] {
		t.Error("Expected sentinel_panics_total")
	}
	if !foundNames["sentinel_retries_total"] {
		t.Error("Expected sentinel_retries_total")
	}
}

func TestConditionalMetrics_WithoutPanicAndRetryOptions_MetricsAbsent(t *testing.T) {
	t.Parallel()

	observer := NewObserver(WithSuccessMetrics())
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}

	for _, f := range families {
		if *f.Name == "sentinel_panics_total" {
			t.Error("sentinel_panics_total should not be registered without WithPanicMetrics()")
		}
		if *f.Name == "sentinel_retries_total" {
			t.Error("sentinel_retries_total should not be registered without WithRetryMetrics()")
		}
	}
}

func TestConditionalMetrics_OnlyDuration_NoNilPanicOnErrorPath(t *testing.T) {
	t.Parallel()

	// Only duration metrics enabled - error path should not nil-panic.
	observer := NewObserver(WithDurationMetrics([]float64{0.1, 1}))

	err := observer.RunFunc(func(_ context.Context) error {
		return errors.New("test error")
	})
	if err == nil {
		t.Error("Expected error to be returned")
	}
	// If we reach here without panicking, the nil-checks are working.
}

func TestMetrics_WithErrorLabels_CounterVec(t *testing.T) {
	t.Parallel()

	t.Run("errorsLabeledVec is non-nil when errorLabelNames are set", func(t *testing.T) {
		t.Parallel()

		cfg := testConfig(t)
		cfg.errorLabelNames = []string{"type"}
		m := newMetrics(cfg)

		if m.errorsLabeledVec == nil {
			t.Fatal("Expected errorsLabeledVec to be non-nil when errorLabelNames are set")
		}
		if m.errors != nil {
			t.Error("Expected plain errors counter to be nil when errorsLabeledVec is used")
		}
	})

	t.Run("errorsLabeledVec supports distinct label values", func(t *testing.T) {
		t.Parallel()

		cfg := testConfig(t)
		cfg.errorLabelNames = []string{"type"}
		m := newMetrics(cfg)
		registry := prometheus.NewRegistry()
		m.MustRegister(registry)

		m.errorsLabeledVec.With(prometheus.Labels{"type": "io_error"}).Inc()
		m.errorsLabeledVec.With(prometheus.Labels{"type": "io_error"}).Inc()
		m.errorsLabeledVec.With(prometheus.Labels{"type": "db_error"}).Inc()

		ioCount := testutil.ToFloat64(m.errorsLabeledVec.With(prometheus.Labels{"type": "io_error"}))
		if ioCount != 2 {
			t.Errorf("Expected io_error count=2, got %f", ioCount)
		}

		dbCount := testutil.ToFloat64(m.errorsLabeledVec.With(prometheus.Labels{"type": "db_error"}))
		if dbCount != 1 {
			t.Errorf("Expected db_error count=1, got %f", dbCount)
		}
	})

	t.Run("errorsLabeledVec is registered in prometheus registry", func(t *testing.T) {
		t.Parallel()

		cfg := testConfig(t)
		cfg.errorLabelNames = []string{"kind"}
		m := newMetrics(cfg)
		registry := prometheus.NewRegistry()
		m.MustRegister(registry)

		families, err := registry.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		metricName := "test_metrics_errors_total"
		found := false
		for _, family := range families {
			if *family.Name == metricName {
				found = true
				break
			}
		}
		// errorsLabeledVec only appears after a label value is observed
		// Increment to trigger registration
		m.errorsLabeledVec.With(prometheus.Labels{"kind": "timeout"}).Inc()
		families, err = registry.Gather()
		if err != nil {
			t.Fatalf("Failed to gather after increment: %v", err)
		}
		for _, family := range families {
			if *family.Name == metricName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected %q to be registered", metricName)
		}
	})
}
