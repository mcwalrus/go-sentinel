// Package main runs a worker-loop example demonstrating the async Submit()/Wait() API
// with Prometheus metrics and graceful shutdown via circuit control.
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	sentinel "github.com/mcwalrus/go-sentinel/v2"
	"github.com/mcwalrus/go-sentinel/v2/circuit"
	"github.com/mcwalrus/go-sentinel/v2/retry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ob           *sentinel.Observer
	registry     *prometheus.Registry
	attemptCount int64
	done         = make(chan struct{})
)

func init() {
	ob = sentinel.NewObserver(
		// WithDurationMetrics tracks per-task execution time as a histogram.
		sentinel.WithDurationMetrics([]float64{0.01, 0.1, 1, 10, 100, 1000, 10_000}),
		sentinel.WithNamespace("example"),
		sentinel.WithSubsystem("workerloop"),
		sentinel.WithDescription("Worker loop"),
		// WithMaxConcurrency caps the goroutine pool to 10 concurrent tasks.
		// Tasks submitted beyond this limit queue until a pool slot is free.
		sentinel.WithMaxConcurrency(10),
		// WithQueueMetrics tracks the number of pending (queued) tasks as a gauge.
		// This is only meaningful when combined with WithMaxConcurrency.
		sentinel.WithQueueMetrics(),
		// WithTimeout applies a 5s deadline to each task attempt via its context.
		sentinel.WithTimeout(5*time.Second),
		// WithRetrier sets observer-level retry logic (up to 2 retries, linear back-off).
		sentinel.WithRetrier(retry.DefaultRetrier{
			WaitStrategy: retry.Linear(1 * time.Second),
			MaxRetries:   2,
		}),
		// WithControl uses circuit.WhenClosed for graceful shutdown:
		// closing the done channel stops new task executions and retries.
		sentinel.WithControl(circuit.WhenClosed(done)),
	)
	registry = prometheus.NewRegistry()
	ob.MustRegister(registry)
}

// workFunc returns a task function for use with SubmitFunc.
// The context it receives already has the observer's 5s timeout applied.
func workFunc(taskID string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		// track attempt number for deterministic failure injection
		currentAttempt := atomic.AddInt64(&attemptCount, 1)
		log.Printf("Starting task %s (attempt #%d)", taskID, currentAttempt)
		if currentAttempt > 100_000 {
			currentAttempt = atomic.SwapInt64(&attemptCount, 0)
		}

		// simulate processing time between 500ms and 5000ms
		processingTime := time.Duration(rand.Intn(4500)+500) * time.Millisecond
		select {
		case <-ctx.Done():
			// context cancelled or timed out; observer records a timeout metric
			return ctx.Err()
		case <-time.After(processingTime):
		}

		// panic on every 10th attempt; observer catches the panic and records it
		if currentAttempt%10 == 0 {
			log.Printf("Task %s triggering panic! (attempt #%d)", taskID, currentAttempt)
			panic(fmt.Sprintf("simulated panic in task %s at attempt %d", taskID, currentAttempt))
		}

		// fail with error on every fourth attempt; observer retries up to 2 times
		if currentAttempt%4 == 0 {
			log.Printf("Task %s failed (attempt #%d)", taskID, currentAttempt)
			return fmt.Errorf("processing failed for task %s at attempt %d", taskID, currentAttempt)
		}

		log.Printf("Completed task %s successfully (attempt #%d, took %v)", taskID, currentAttempt, processingTime)
		return nil
	}
}

func run() {
	taskID := 1
	burstCycle := 0

	log.Println("Starting task processing with burst periods...")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		burstCycle++

		var tasksToCreate int
		if burstCycle%3 == 0 {
			// Burst period: create 8-12 tasks
			tasksToCreate = rand.Intn(5) + 8
			log.Printf("BURST PERIOD: Creating %d tasks (cycle %d)", tasksToCreate, burstCycle)
		} else {
			// Normal period: create 1-3 tasks
			tasksToCreate = rand.Intn(3) + 1
			log.Printf("Normal period: Creating %d tasks (cycle %d)", tasksToCreate, burstCycle)
		}

		// --- Async dispatch: submit all tasks for this batch without blocking ---
		// SubmitFunc enqueues each task into the observer's worker pool.
		// WithMaxConcurrency(10) caps concurrent goroutines; any extras queue up
		// and are visible in the pending_total gauge (enabled by WithQueueMetrics).
		// Prometheus metrics at :8080/metrics reflect live state while tasks run.
		for i := 0; i < tasksToCreate; i++ {
			id := fmt.Sprintf("task-%04d", taskID)
			taskID++
			ob.SubmitFunc(workFunc(id))
		}

		// --- Wait drains the pool and returns all collected errors joined together ---
		// The observer is reusable after Wait returns, so the next cycle can
		// submit a new batch immediately. This structures execution as discrete
		// batch cycles: submit N tasks, drain, handle errors, repeat.
		if err := ob.Wait(); err != nil {
			log.Printf("Cycle %d completed with errors: %v", burstCycle, err)
		} else {
			log.Printf("Cycle %d completed successfully", burstCycle)
		}
	}
}

func main() {
	metricsServer := &http.Server{
		Addr:    ":8080",
		Handler: promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}

	// Prometheus metrics are scraped at http://localhost:8080/metrics.
	// Scraping during task execution shows in_flight, pending_total, durations, etc.
	go func() {
		log.Println("Starting metrics server on :8080/metrics")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Metrics server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping gracefully...")
		// Close done to stop new tasks and retries via WithControl(circuit.WhenClosed(done)).
		close(done)
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("Error shutting down metrics server: %v", err)
		}
		os.Exit(0)
	}()

	log.Println("Worker loop running... Press Ctrl+C to stop")

	run()
}
