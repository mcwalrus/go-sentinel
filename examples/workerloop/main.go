package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	sentinel "github.com/mcwalrus/go-sentinel"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ob       *sentinel.Observer
	registry *prometheus.Registry
)

func init() {
	ob = sentinel.NewObserver(sentinel.ObserverConfig{
		Namespace:   "example",
		Subsystem:   "workerloop",
		Description: "Worker loop",
		Buckets:     []float64{0.01, 0.1, 1, 10, 100, 1000, 10_000},
	})

	// Create a custom registry and register our observer metrics
	registry = prometheus.NewRegistry()
	ob.MustRegister(registry)
}

// Task represents a processing task with various outcomes
type ProcessingTask struct {
	TaskID      string
	Operation   string
	Payload     map[string]interface{}
	ShouldPanic bool
	ShouldFail  bool
}

func (task *ProcessingTask) Execute(ctx context.Context) error {
	log.Printf("Starting task %s: %s", task.TaskID, task.Operation)

	// Simulate variable processing time (50ms to 3s)
	processingTime := time.Duration(rand.Intn(2950)+50) * time.Millisecond

	// Check for context cancellation during processing
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingTime / 3):
		// Continue processing
	}

	// Simulate panic scenarios (caught by sentinel)
	if task.ShouldPanic {
		log.Printf("Task %s triggering panic!", task.TaskID)
		panic(fmt.Sprintf("simulated panic in task %s", task.TaskID))
	}

	// Simulate processing work
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingTime * 2 / 3):
		// Continue processing
	}

	// Simulate failure scenarios
	if task.ShouldFail {
		log.Printf("Task %s failed with error", task.TaskID)
		return fmt.Errorf("processing failed for task %s: %s", task.TaskID, task.Operation)
	}

	log.Printf("Completed task %s (took %v)", task.TaskID, processingTime)
	return nil
}

func run() {
	// Create initial batch of tasks with various scenarios
	initialTasks := createTaskBatch("batch-1", 12)

	log.Printf("Starting %d initial tasks concurrently", len(initialTasks))

	// Launch all initial tasks concurrently to demonstrate multiple in-flight requests
	for _, task := range initialTasks {
		currentTask := task // Capture for closure
		ob.Run(sentinel.TaskConfig{
			Concurrent:    true,
			RecoverPanics: true,
			MaxRetries:    2,
			RetryStrategy: sentinel.RetryStrategyExponentialBackoff(100 * time.Millisecond),
			Timeout:       3 * time.Second,
		}, func(ctx context.Context) error {
			return currentTask.Execute(ctx)
		})
	}

	// Let initial batch process
	time.Sleep(2 * time.Second)

	// Add more tasks periodically to maintain load
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	batchID := 2
	for range ticker.C {
		// Create smaller batches periodically
		batchTasks := createTaskBatch(fmt.Sprintf("batch-%d", batchID), 4)
		batchID++

		log.Printf("Adding batch of %d tasks", len(batchTasks))

		for _, task := range batchTasks {
			currentTask := task // Capture for closure
			ob.Run(sentinel.TaskConfig{
				Concurrent:    true,
				RecoverPanics: true,
				MaxRetries:    1,
				RetryStrategy: sentinel.RetryStrategyLinearBackoff(200 * time.Millisecond),
				Timeout:       4 * time.Second,
			}, func(ctx context.Context) error {
				return currentTask.Execute(ctx)
			})
		}
	}
}

// createTaskBatch generates a batch of tasks with realistic scenarios
func createTaskBatch(batchID string, count int) []*ProcessingTask {
	operations := []string{
		"process-payment", "send-notification", "generate-report",
		"sync-database", "validate-user", "update-inventory",
		"backup-data", "analyze-metrics", "compress-files",
		"send-email", "update-cache", "process-image",
	}

	tasks := make([]*ProcessingTask, count)

	for i := 0; i < count; i++ {
		// Create realistic failure and panic scenarios
		failureRate := 0.15 // 15% failure rate
		panicRate := 0.08   // 8% panic rate

		tasks[i] = &ProcessingTask{
			TaskID:    fmt.Sprintf("%s-task-%03d", batchID, i+1),
			Operation: operations[rand.Intn(len(operations))],
			Payload: map[string]interface{}{
				"user_id":   rand.Intn(10000),
				"timestamp": time.Now().Unix(),
				"priority":  []string{"low", "medium", "high"}[rand.Intn(3)],
				"size":      rand.Intn(1000) + 100,
			},
			ShouldFail:  rand.Float64() < failureRate,
			ShouldPanic: rand.Float64() < panicRate,
		}
	}

	return tasks
}

func main() {
	// Start metrics server in a goroutine
	metricsServer := &http.Server{
		Addr:    ":8080",
		Handler: promhttp.HandlerFor(registry, promhttp.HandlerOpts{}),
	}

	go func() {
		log.Println("Starting metrics server on :8080/metrics")
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Metrics server error: %v", err)
		}
	}()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping gracefully...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		// Shutdown metrics server
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error shutting down metrics server: %v", err)
		}
	}()

	// Keep running until shutdown signal
	log.Println("Worker loop running... Press Ctrl+C to stop")

	run()
}
