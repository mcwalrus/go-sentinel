package main

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	sentinel "github.com/mcwalrus/go-sentinel"
	"github.com/mcwalrus/go-sentinel/retry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	vecObserver := sentinel.NewVecObserver(
		[]float64{0.01, 0.1, 0.5, 1, 2, 5},
		[]string{"service", "pipeline"},
		sentinel.WithNamespace("example"),
		sentinel.WithSubsystem("vec"),
	)

	// Register VecObserver metrics
	registry := prometheus.NewRegistry()
	vecObserver.MustRegister(registry)

	mObserver, err := vecObserver.WithLabels("api", "main")
	if err != nil {
		log.Fatalf("Failed to create main observer: %v", err)
	}
	bObserver, err := vecObserver.WithLabels("api", "background")
	if err != nil {
		log.Fatalf("Failed to create background observer: %v", err)
	}
	dbObserver, err := vecObserver.WithLabels("database", "read-write")
	if err != nil {
		log.Fatalf("Failed to create database observer: %v", err)
	}

	// Set configurations for each observer
	mObserver.UseConfig(sentinel.ObserverConfig{
		Timeout:       5 * time.Second,
		MaxRetries:    2,
		RetryStrategy: retry.Exponential(100 * time.Millisecond),
	})

	bObserver.UseConfig(sentinel.ObserverConfig{
		Timeout:       10 * time.Second,
		MaxRetries:    3,
		RetryStrategy: retry.Linear(200 * time.Millisecond),
	})

	dbObserver.UseConfig(sentinel.ObserverConfig{
		Timeout:    2 * time.Second,
		MaxRetries: 1,
	})

	// Start metrics server
	metricsServer := &http.Server{
		Addr:    ":8080",
		Handler: promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}

	go func() {
		log.Println("Metrics server running on :8080/metrics")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Metrics server error: %v", err)
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := metricsServer.Shutdown(ctx); err != nil {
			log.Fatalf("Error shutting down: %v", err)
		}
		os.Exit(0)
	}()

	// Simulate tasks running with different observers
	log.Println("Running tasks with VecObserver...")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	taskCount := 0
	for range ticker.C {
		taskCount++

		// Simulate main tasks
		go func() {
			err := mObserver.RunFunc(func(ctx context.Context) error {
				time.Sleep(time.Duration(rand.Intn(500)) * time.Millisecond)
				if rand.Float32() < 0.2 {
					return errors.New("api production error")
				}
				return nil
			})
			if err != nil {
				log.Printf("[api-prod] Task failed: %v", err)
			}
		}()

		// Simulate background tasks
		go func() {
			err := bObserver.RunFunc(func(ctx context.Context) error {
				time.Sleep(time.Duration(rand.Intn(800)) * time.Millisecond)
				if rand.Float32() < 0.3 {
					return errors.New("api staging error")
				}
				return nil
			})
			if err != nil {
				log.Printf("[api-staging] Task failed: %v", err)
			}
		}()

		// Simulate database read-write tasks (less frequent)
		if taskCount%3 == 0 {
			go func() {
				err := dbObserver.RunFunc(func(ctx context.Context) error {
					time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)
					if rand.Float32() < 0.15 {
						return errors.New("database production error")
					}
					return nil
				})
				if err != nil {
					log.Printf("[db-prod] Task failed: %v", err)
				}
			}()
		}

		if taskCount%10 == 0 {
			log.Printf("Processed %d task cycles. Check metrics at http://localhost:8080/metrics", taskCount)
		}
	}
}
