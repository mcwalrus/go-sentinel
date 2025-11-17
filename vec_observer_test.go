package sentinel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestVecObserver_Fork_IndividualMetrics(t *testing.T) {
	t.Parallel()

	t.Run("forked observers record metrics individually", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			[]float64{0.1, 0.5, 1, 2, 5},
			[]string{"service", "pipeline"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.WithLabels("api", "main")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.WithLabels("db", "users")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		err = child1.Run(func() error {
			return nil
		})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		err = child1.Run(func() error {
			return nil
		})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		err = child2.Run(func() error {
			return errors.New("database error")
		})
		if err == nil {
			t.Fatal("Expected error, got nil")
		}

		if got := testutil.ToFloat64(child1.metrics.successes); got != 2 {
			t.Errorf("child1 successes: expected 2, got %f", got)
		}
		if got := testutil.ToFloat64(child1.metrics.failures); got != 0 {
			t.Errorf("child1 failures: expected 0, got %f", got)
		}
		if got := testutil.ToFloat64(child1.metrics.errors); got != 0 {
			t.Errorf("child1 errors: expected 0, got %f", got)
		}

		if got := testutil.ToFloat64(child2.metrics.successes); got != 0 {
			t.Errorf("child2 successes: expected 0, got %f", got)
		}
		if got := testutil.ToFloat64(child2.metrics.failures); got != 1 {
			t.Errorf("child2 failures: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(child2.metrics.errors); got != 1 {
			t.Errorf("child2 errors: expected 1, got %f", got)
		}
	})

	t.Run("forked observers record metrics with correct labels in vecMetrics", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			[]float64{0.1, 0.5, 1, 2, 5},
			[]string{"service", "pipeline"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.WithLabels("api", "main")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.WithLabels("db", "users")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child3, err := vecObserver.WithLabels("cache", "hot")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		_ = child1.Run(func() error { return nil })
		_ = child1.Run(func() error { return nil })
		_ = child2.Run(func() error { return errors.New("db error") })
		_ = child3.Run(func() error { return nil })

		if got := testutil.ToFloat64(vecObserver.metrics.successesVec.WithLabelValues("api", "main")); got != 2 {
			t.Errorf("vecMetrics[api,success] successes: expected 2, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.failuresVec.WithLabelValues("api", "main")); got != 0 {
			t.Errorf("vecMetrics[api,success] failures: expected 0, got %f", got)
		}

		if got := testutil.ToFloat64(vecObserver.metrics.successesVec.WithLabelValues("db", "users")); got != 0 {
			t.Errorf("vecMetrics[db,error] successes: expected 0, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.failuresVec.WithLabelValues("db", "users")); got != 1 {
			t.Errorf("vecMetrics[db,error] failures: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.errorsVec.WithLabelValues("db", "users")); got != 1 {
			t.Errorf("vecMetrics[db,error] errors: expected 1, got %f", got)
		}

		if got := testutil.ToFloat64(vecObserver.metrics.successesVec.WithLabelValues("cache", "hot")); got != 1 {
			t.Errorf("vecMetrics[cache,success] successes: expected 1, got %f", got)
		}
	})

	t.Run("multiple forked observers aggregate correctly in vecMetrics", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			nil,
			[]string{"service"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child3, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		_ = child1.Run(func() error { return nil })
		_ = child2.Run(func() error { return nil })
		_ = child3.Run(func() error { return errors.New("users") })

		if got := testutil.ToFloat64(vecObserver.metrics.successesVec.WithLabelValues("api")); got != 2 {
			t.Errorf("vecMetrics[api] successes: expected 2, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.failuresVec.WithLabelValues("api")); got != 1 {
			t.Errorf("vecMetrics[api] failures: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.errorsVec.WithLabelValues("api")); got != 1 {
			t.Errorf("vecMetrics[api] errors: expected 1, got %f", got)
		}
	})

	t.Run("forked observers handle panics individually", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			nil,
			[]string{"service"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.WithLabels("db")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		err = child1.Run(func() error {
			panic("test panic")
		})
		if err == nil {
			t.Fatal("Expected error from panic, got nil")
		}

		err = child2.Run(func() error {
			return nil
		})
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}

		if got := testutil.ToFloat64(child1.metrics.panics); got != 1 {
			t.Errorf("child1 panics: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(child1.metrics.errors); got != 1 {
			t.Errorf("child1 errors: expected 1, got %f", got)
		}

		if got := testutil.ToFloat64(child2.metrics.panics); got != 0 {
			t.Errorf("child2 panics: expected 0, got %f", got)
		}
		if got := testutil.ToFloat64(child2.metrics.successes); got != 1 {
			t.Errorf("child2 successes: expected 1, got %f", got)
		}

		if got := testutil.ToFloat64(vecObserver.metrics.panicsVec.WithLabelValues("api")); got != 1 {
			t.Errorf("vecMetrics[api] panics: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.panicsVec.WithLabelValues("db")); got != 0 {
			t.Errorf("vecMetrics[db] panics: expected 0, got %f", got)
		}
	})

	t.Run("forked observers handle retries individually", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			nil,
			[]string{"service"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.WithLabels("db")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		child1.UseConfig(ObserverConfig{
			MaxRetries: 2,
		})
		child2.UseConfig(ObserverConfig{
			MaxRetries: 1,
		})

		attemptCount1 := 0
		err1 := child1.RunFunc(func(ctx context.Context) error {
			attemptCount1++
			if attemptCount1 < 3 {
				return errors.New("retryable error")
			}
			return nil
		})
		if err1 != nil {
			t.Fatalf("child1: expected success after retries, got %v", err1)
		}

		attemptCount2 := 0
		err2 := child2.RunFunc(func(ctx context.Context) error {
			attemptCount2++
			return errors.New("permanent error")
		})
		if err2 == nil {
			t.Fatal("child2: expected error after retries exhausted, got nil")
		}

		if got := testutil.ToFloat64(child1.metrics.retries); got != 2 {
			t.Errorf("child1 retries: expected 2, got %f", got)
		}
		if got := testutil.ToFloat64(child1.metrics.successes); got != 1 {
			t.Errorf("child1 successes: expected 1, got %f", got)
		}

		if got := testutil.ToFloat64(child2.metrics.retries); got != 1 {
			t.Errorf("child2 retries: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(child2.metrics.failures); got != 1 {
			t.Errorf("child2 failures: expected 1, got %f", got)
		}

		if got := testutil.ToFloat64(vecObserver.metrics.retriesVec.WithLabelValues("api")); got != 2 {
			t.Errorf("vecMetrics[api] retries: expected 2, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.retriesVec.WithLabelValues("db")); got != 1 {
			t.Errorf("vecMetrics[db] retries: expected 1, got %f", got)
		}
	})

	t.Run("forked observers handle timeouts individually", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			nil,
			[]string{"service"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.WithLabels("db")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		child1.UseConfig(ObserverConfig{
			Timeout: 50 * time.Millisecond,
		})

		child2.UseConfig(ObserverConfig{
			Timeout: 200 * time.Millisecond,
		})

		err1 := child1.RunFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return nil
			}
		})
		if err1 == nil || !errors.Is(err1, context.DeadlineExceeded) {
			t.Fatalf("child1: expected DeadlineExceeded, got %v", err1)
		}

		err2 := child2.RunFunc(func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return nil
			}
		})
		if err2 != nil {
			t.Fatalf("child2: expected no error, got %v", err2)
		}

		if got := testutil.ToFloat64(child1.metrics.timeouts); got != 1 {
			t.Errorf("child1 timeouts: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(child1.metrics.errors); got != 1 {
			t.Errorf("child1 errors: expected 1, got %f", got)
		}

		if got := testutil.ToFloat64(child2.metrics.timeouts); got != 0 {
			t.Errorf("child2 timeouts: expected 0, got %f", got)
		}
		if got := testutil.ToFloat64(child2.metrics.successes); got != 1 {
			t.Errorf("child2 successes: expected 1, got %f", got)
		}

		if got := testutil.ToFloat64(vecObserver.metrics.timeoutsVec.WithLabelValues("api")); got != 1 {
			t.Errorf("vecMetrics[api] timeouts: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.timeoutsVec.WithLabelValues("db")); got != 0 {
			t.Errorf("vecMetrics[db] timeouts: expected 0, got %f", got)
		}
	})

	t.Run("forked observers record durations individually", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			[]float64{0.01, 0.1, 1, 10},
			[]string{"service"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.WithLabels("db")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		_ = child1.RunFunc(func(ctx context.Context) error {
			time.Sleep(50 * time.Millisecond)
			return nil
		})

		_ = child2.RunFunc(func(ctx context.Context) error {
			time.Sleep(150 * time.Millisecond)
			return nil
		})

		families, err := registry.Gather()
		if err != nil {
			t.Fatalf("Failed to gather metrics: %v", err)
		}

		var apiSampleCount, dbSampleCount uint64
		for _, family := range families {
			if *family.Name == "sentinel_durations_seconds" {
				for _, metric := range family.Metric {
					if len(metric.Label) == 1 && metric.Label[0].GetName() == "service" {
						if metric.Label[0].GetValue() == "api" && metric.Histogram != nil {
							apiSampleCount = metric.Histogram.GetSampleCount()
						}
						if metric.Label[0].GetValue() == "db" && metric.Histogram != nil {
							dbSampleCount = metric.Histogram.GetSampleCount()
						}
					}
				}
			}
		}

		if apiSampleCount != 1 {
			t.Errorf("vecMetrics[api] durations: expected 1 sample, got %d", apiSampleCount)
		}
		if dbSampleCount != 1 {
			t.Errorf("vecMetrics[db] durations: expected 1 sample, got %d", dbSampleCount)
		}
	})

	t.Run("forked observers with ForkWith method", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(
			nil,
			[]string{"service", "environment"},
		)
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		child1, err := vecObserver.With(prometheus.Labels{
			"service":     "api",
			"environment": "production",
		})
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		child2, err := vecObserver.With(prometheus.Labels{
			"service":     "api",
			"environment": "staging",
		})
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}

		_ = child1.Run(func() error { return nil })
		_ = child2.Run(func() error { return errors.New("users") })

		if got := testutil.ToFloat64(child1.metrics.successes); got != 1 {
			t.Errorf("child1 successes: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(child2.metrics.failures); got != 1 {
			t.Errorf("child2 failures: expected 1, got %f", got)
		}

		time.Sleep(10 * time.Millisecond)
		if got := testutil.ToFloat64(vecObserver.metrics.successesVec.WithLabelValues("api", "production")); got != 1 {
			t.Errorf("vecMetrics[api,production] successes: expected 1, got %f", got)
		}
		if got := testutil.ToFloat64(vecObserver.metrics.failuresVec.WithLabelValues("api", "staging")); got != 1 {
			t.Errorf("vecMetrics[api,staging] failures: expected 1, got %f", got)
		}
	})
}

func TestVecObserver_Describe(t *testing.T) {
	t.Parallel()

	t.Run("describes all vec metrics without duration buckets", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(nil, []string{"service"})
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		// Create a forked observer and run a task to ensure metrics are initialized
		child, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		_ = child.Run(func() error {
			return nil
		})

		ch := make(chan *prometheus.Desc, 10)

		go func() {
			vecObserver.Describe(ch)
			close(ch)
		}()

		descs := make(map[string]*prometheus.Desc)
		for desc := range ch {
			if desc != nil {
				descs[desc.String()] = desc
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
		}

		if len(descs) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metric descriptions, got %d", len(expectedMetrics), len(descs))
		}

		descStrings := make([]string, 0, len(descs))
		for descStr := range descs {
			descStrings = append(descStrings, descStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, descStr := range descStrings {
				if strings.Contains(descStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find description for metric %s", expectedName)
			}
		}
	})

	t.Run("describes all vec metrics with duration buckets", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver([]float64{0.1, 0.5, 1, 2, 5}, []string{"service", "environment"})
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		// Create a forked observer and run a task to ensure metrics are initialized
		child, err := vecObserver.WithLabels("api", "production")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		_ = child.Run(func() error {
			return nil
		})

		ch := make(chan *prometheus.Desc, 10)

		go func() {
			vecObserver.Describe(ch)
			close(ch)
		}()

		descs := make(map[string]*prometheus.Desc)
		for desc := range ch {
			if desc != nil {
				descs[desc.String()] = desc
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
			"sentinel_durations_seconds",
		}

		if len(descs) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metric descriptions, got %d", len(expectedMetrics), len(descs))
		}

		descStrings := make([]string, 0, len(descs))
		for descStr := range descs {
			descStrings = append(descStrings, descStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, descStr := range descStrings {
				if strings.Contains(descStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find description for metric %s", expectedName)
			}
		}
	})
}

func TestVecObserver_Collect(t *testing.T) {
	t.Parallel()

	t.Run("collects all vec metrics without duration buckets", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver(nil, []string{"service"})
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		// Create a forked observer and run a task to ensure metrics are initialized
		child, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		_ = child.Run(func() error {
			return nil
		})

		ch := make(chan prometheus.Metric, 10)

		go func() {
			vecObserver.Collect(ch)
			close(ch)
		}()

		metrics := make(map[string]prometheus.Metric)
		for metric := range ch {
			if metric != nil {
				desc := metric.Desc()
				metrics[desc.String()] = metric
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
		}

		if len(metrics) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metrics, got %d", len(expectedMetrics), len(metrics))
		}

		metricStrings := make([]string, 0, len(metrics))
		for metricStr := range metrics {
			metricStrings = append(metricStrings, metricStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, metricStr := range metricStrings {
				if strings.Contains(metricStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find metric %s", expectedName)
			}
		}
	})

	t.Run("collects all vec metrics with duration buckets", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver([]float64{0.1, 0.5, 1, 2, 5}, []string{"service", "environment"})
		registry := prometheus.NewRegistry()
		vecObserver.MustRegister(registry)

		// Create a forked observer and run a task to ensure metrics are initialized
		child, err := vecObserver.WithLabels("api", "production")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		_ = child.Run(func() error {
			return nil
		})

		ch := make(chan prometheus.Metric, 10)

		go func() {
			vecObserver.Collect(ch)
			close(ch)
		}()

		metrics := make(map[string]prometheus.Metric)
		for metric := range ch {
			if metric != nil {
				desc := metric.Desc()
				metrics[desc.String()] = metric
			}
		}

		expectedMetrics := []string{
			"sentinel_in_flight",
			"sentinel_success_total",
			"sentinel_failures_total",
			"sentinel_errors_total",
			"sentinel_panics_total",
			"sentinel_timeouts_total",
			"sentinel_retries_total",
			"sentinel_durations_seconds",
		}

		if len(metrics) < len(expectedMetrics) {
			t.Errorf("Expected at least %d metrics, got %d", len(expectedMetrics), len(metrics))
		}

		metricStrings := make([]string, 0, len(metrics))
		for metricStr := range metrics {
			metricStrings = append(metricStrings, metricStr)
		}

		for _, expectedName := range expectedMetrics {
			found := false
			for _, metricStr := range metricStrings {
				if strings.Contains(metricStr, expectedName) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find metric %s", expectedName)
			}
		}
	})

	t.Run("collects metrics after task execution", func(t *testing.T) {
		t.Parallel()

		vecObserver := NewVecObserver([]float64{0.1, 0.5, 1, 2, 5}, []string{"service"})
		child, err := vecObserver.WithLabels("api")
		if err != nil {
			t.Fatalf("Failed to create forked observer: %v", err)
		}
		_ = child.Run(func() error {
			return nil
		})

		ch := make(chan prometheus.Metric, 10)
		go func() {
			vecObserver.Collect(ch)
			close(ch)
		}()

		metrics := make(map[string]prometheus.Metric)
		for metric := range ch {
			if metric != nil {
				desc := metric.Desc()
				metrics[desc.String()] = metric
			}
		}

		if len(metrics) == 0 {
			t.Error("Expected to collect at least one metric after task execution")
		}
	})
}
