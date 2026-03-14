package sentinel_test

import (
	"context"
	"fmt"
	"log"

	sentinel "github.com/mcwalrus/go-sentinel"
	"github.com/prometheus/client_golang/prometheus"
)

func ExampleNewObserverDefault() {
	registry := prometheus.NewRegistry()
	observer := sentinel.NewObserverDefault(
		sentinel.WithNamespace("myapp"),
		sentinel.WithSubsystem("worker"),
	)
	observer.MustRegister(registry)

	err := observer.RunFunc(func(_ context.Context) error {
		fmt.Println("hello from sentinel")
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	// Output:
	// hello from sentinel
}
