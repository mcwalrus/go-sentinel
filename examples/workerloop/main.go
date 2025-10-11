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

	sentinel "github.com/mcwalrus/go-sentinel"
	"github.com/mcwalrus/go-sentinel/retry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ob           *sentinel.Observer
	registry     *prometheus.Registry
	limitChan    chan struct{}
	attemptCount int64
)

func init() {
	ob = sentinel.NewObserver(
		sentinel.WithNamespace("example"),
		sentinel.WithSubsystem("workerloop"),
		sentinel.WithDescription("Worker loop"),
		sentinel.WithDurationMetrics([]float64{0.01, 0.1, 1, 10, 100, 1000, 10_000}),
	)
	registry = prometheus.NewRegistry()
	ob.MustRegister(registry)
	limitChan = make(chan struct{}, 10)
}

// Task represents a processing task, implements sentinel.Task
type ProcessingTask struct {
	TaskID string
}

func (task *ProcessingTask) Config() sentinel.Config {
	return sentinel.Config{
		Timeout:       5 * time.Second,
		MaxRetries:    2,
		RetryStrategy: retry.Linear(1 * time.Second),
	}
}

func (task *ProcessingTask) Execute(ctx context.Context) error {
	limitChan <- struct{}{}
	defer func() {
		<-limitChan
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	timeoutCtx, cancelTimeout := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancelTimeout()

	// find current attempt number
	currentAttempt := atomic.AddInt64(&attemptCount, 1)
	log.Printf("Starting task %s (attempt #%d)", task.TaskID, currentAttempt)
	if currentAttempt > 100_000 {
		currentAttempt = atomic.SwapInt64(&attemptCount, 0)
	}

	// simulate processing time between 500ms to 5000ms
	processingTime := time.Duration(rand.Intn(4500)+500) * time.Millisecond
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingTime):
	}

	// panic on every 10th attempt
	if currentAttempt%10 == 0 {
		log.Printf("Task %s triggering panic! (attempt #%d)", task.TaskID, currentAttempt)
		panic(fmt.Sprintf("simulated panic in task %s at attempt %d", task.TaskID, currentAttempt))
	}

	// timeout on every seventh attempt
	if currentAttempt%7 == 0 {
		time.Sleep(1 * time.Millisecond)
		log.Printf("Task %s timed out (attempt #%d)", task.TaskID, currentAttempt)
		return fmt.Errorf("processing timeout for task %s at attempt %d: %w", task.TaskID, currentAttempt, timeoutCtx.Err())
	}

	// fail with error on every fourth attempt
	if currentAttempt%4 == 0 {
		log.Printf("Task %s failed (attempt #%d)", task.TaskID, currentAttempt)
		return fmt.Errorf("processing failed for task %s at attempt %d", task.TaskID, currentAttempt)
	}

	// Otherwise, succeed on every other attempt
	log.Printf("Completed task %s successfully (attempt #%d, took %v)", task.TaskID, currentAttempt, processingTime)
	return nil
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

		for i := 0; i < tasksToCreate; i++ {
			task := &ProcessingTask{
				TaskID: fmt.Sprintf("task-%04d", taskID),
			}
			taskID++
			ob.RunTask(task)
		}
	}
}

func main() {
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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping gracefully...")
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
