// Package main runs a vector-observer example with sentinel and Prometheus metrics.
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
	commonOpts := []sentinel.ObserverOption{
		sentinel.WithDurationMetrics([]float64{0.01, 0.1, 0.5, 1, 2, 5}),
		sentinel.WithNamespace("example"),
		sentinel.WithSubsystem("vec"),
	}

	// Create a VecObserver for API main tasks with its own config.
	mVec := sentinel.NewVecObserver(
		[]string{"service", "pipeline"},
		append(commonOpts,
			sentinel.WithTimeout(5*time.Second),
			sentinel.WithRetrier(retry.DefaultRetrier{
				MaxRetries:   2,
				WaitStrategy: retry.Exponential(100 * time.Millisecond),
			}),
		)...,
	)

	// Create a VecObserver for API background tasks with its own config.
	bVec := sentinel.NewVecObserver(
		[]string{"service", "pipeline"},
		append(commonOpts,
			sentinel.WithTimeout(10*time.Second),
			sentinel.WithRetrier(retry.DefaultRetrier{
				MaxRetries:   3,
				WaitStrategy: retry.Linear(200 * time.Millisecond),
			}),
		)...,
	)

	// Create a VecObserver for database tasks with its own config.
	dbVec := sentinel.NewVecObserver(
		[]string{"service", "pipeline"},
		append(commonOpts,
			sentinel.WithTimeout(2*time.Second),
			sentinel.WithRetrier(retry.DefaultRetrier{MaxRetries: 1}),
		)...,
	)

	// Register each VecObserver's metrics with its own registry.
	registry := prometheus.NewRegistry()
	mVec.MustRegister(registry)
	bVec.MustRegister(registry)
	dbVec.MustRegister(registry)

	mObserver, err := mVec.WithLabels("api", "main")
	if err != nil {
		log.Fatalf("Failed to create main observer: %v", err)
	}
	bObserver, err := bVec.WithLabels("api", "background")
	if err != nil {
		log.Fatalf("Failed to create background observer: %v", err)
	}
	dbObserver, err := dbVec.WithLabels("database", "read-write")
	if err != nil {
		log.Fatalf("Failed to create database observer: %v", err)
	}

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
			err := mObserver.RunFunc(func(_ context.Context) error {
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
			err := bObserver.RunFunc(func(_ context.Context) error {
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
				err := dbObserver.RunFunc(func(_ context.Context) error {
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
