package sentinel

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func Benchmark_ObserverRun(b *testing.B) {
	observer := NewObserver(
		[]float64{0.01, 0.1, 1, 10, 100},
		WithNamespace("test"),
		WithSubsystem("metrics"),
		WithDescription("test operations"),
	)
	registry := prometheus.NewRegistry()
	observer.MustRegister(registry)

	b.Run("simple function", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = observer.Run(func() error {
				return nil
			})
		}
	})

	b.Run("function with work", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = observer.Run(func() error {
				time.Sleep(time.Microsecond)
				return nil
			})
		}
	})
}
