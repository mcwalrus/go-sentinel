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
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// Multiple observers for different task types
	backgroundObserver *sentinel.Observer
	criticalObserver   *sentinel.Observer
	apiObserver        *sentinel.Observer

	registry     *prometheus.Registry
	limitChan    chan struct{}
	attemptCount int64
)

func init() {
	// Background tasks observer
	backgroundObserver = sentinel.NewObserver(
		sentinel.WithNamespace("app"),
		sentinel.WithSubsystem("background_tasks"),
		sentinel.WithDescription("Background processing tasks"),
		sentinel.WithBucketDurations([]float64{0.1, 0.5, 1, 2, 5, 10, 30, 60}),
	)

	// Critical tasks observer
	criticalObserver = sentinel.NewObserver(
		sentinel.WithNamespace("app"),
		sentinel.WithSubsystem("critical_tasks"),
		sentinel.WithDescription("Critical business operations"),
		sentinel.WithBucketDurations([]float64{0.01, 0.1, 0.5, 1, 5, 10, 30}),
	)

	// API tasks observer
	apiObserver = sentinel.NewObserver(
		sentinel.WithNamespace("app"),
		sentinel.WithSubsystem("api_requests"),
		sentinel.WithDescription("API request processing"),
		sentinel.WithBucketDurations([]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}),
	)

	limitChan = make(chan struct{}, 15)
	registry = prometheus.NewRegistry()

	backgroundObserver.MustRegister(registry)
	criticalObserver.MustRegister(registry)
	apiObserver.MustRegister(registry)

}

// BackgroundTask represents a long-running background processing task
type BackgroundTask struct {
	TaskID string
}

func (task *BackgroundTask) Config() sentinel.TaskConfig {
	return sentinel.TaskConfig{
		Timeout:       30 * time.Second,
		RecoverPanics: true,
		MaxRetries:    3,
		RetryStrategy: sentinel.RetryStrategyExponentialBackoff(500 * time.Millisecond),
	}
}

// Execute background tasks:
// - Simulate background processing (2-10 seconds)
// - Fail occasionally to show retry behavior
// - Complete the task successfully
func (task *BackgroundTask) Execute(ctx context.Context) error {
	limitChan <- struct{}{}
	defer func() { <-limitChan }()

	currentAttempt := atomic.AddInt64(&attemptCount, 1)
	log.Printf("[BACKGROUND] Starting task %s (attempt #%d)", task.TaskID, currentAttempt)
	processingTime := time.Duration(rand.Intn(8000)+2000) * time.Millisecond

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingTime):
	}
	if currentAttempt%15 == 0 {
		log.Printf("[BACKGROUND] Task %s failed (attempt #%d)", task.TaskID, currentAttempt)
		return fmt.Errorf("background processing failed for task %s", task.TaskID)
	}

	log.Printf(
		"[BACKGROUND] Completed task %s successfully (attempt #%d, took %v)", task.TaskID, currentAttempt, processingTime,
	)
	return nil
}

type CriticalTask struct {
	TaskID string
}

func (task *CriticalTask) Config() sentinel.TaskConfig {
	return sentinel.TaskConfig{
		Timeout:       5 * time.Second,
		RecoverPanics: true,
		MaxRetries:    2,
		RetryStrategy: sentinel.RetryStrategyImmediate,
	}
}

// Execute critical tasks:
// - Simulate critical processing (100ms-2s)
// - Panic occasionally to show panic recovery
// - Timeout occasionally to show timeout handling
// - Complete the task successfully
func (task *CriticalTask) Execute(ctx context.Context) error {
	currentAttempt := atomic.AddInt64(&attemptCount, 1)
	log.Printf("[CRITICAL] Starting task %s (attempt #%d)", task.TaskID, currentAttempt)
	processingTime := time.Duration(rand.Intn(1900)+100) * time.Millisecond

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingTime):
	}

	if currentAttempt%25 == 0 {
		log.Printf("[CRITICAL] Task %s triggering panic! (attempt #%d)", task.TaskID, currentAttempt)
		panic("simulated panic in critical task")
	}
	if currentAttempt%12 == 0 {
		<-ctx.Done() // Force timeout
		return ctx.Err()
	}

	log.Printf(
		"[CRITICAL] Completed task %s successfully (attempt #%d, took %v)",
		task.TaskID, currentAttempt, processingTime,
	)

	return nil
}

type APITask struct {
	TaskID string
}

func (task *APITask) Config() sentinel.TaskConfig {
	return sentinel.TaskConfig{
		Timeout:       2 * time.Second,
		RecoverPanics: true,
	}
}

// Execute API tasks:
// - Simulate API processing (10-500ms)
// - Fails occasionally
// - Complete the task successfully
func (task *APITask) Execute(ctx context.Context) error {
	limitChan <- struct{}{}
	defer func() { <-limitChan }()

	currentAttempt := atomic.AddInt64(&attemptCount, 1)
	log.Printf("[API] Starting task %s (attempt #%d)", task.TaskID, currentAttempt)
	processingTime := time.Duration(rand.Intn(490)+10) * time.Millisecond

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(processingTime):
	}
	if currentAttempt%20 == 0 {
		log.Printf("[API] Task %s failed (attempt #%d)", task.TaskID, currentAttempt)
		return fmt.Errorf("API processing failed for task %s", task.TaskID)
	}

	log.Printf(
		"[API] Completed task %s successfully (attempt #%d, took %v)",
		task.TaskID, currentAttempt, processingTime,
	)

	return nil
}

func run() {
	taskID := 1
	cycle := 0

	log.Println("Starting multi-observer task processing...")

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		cycle++

		// Create different types of tasks based on cycle
		// 0: Background task burst
		// 1: Critical task burst
		// 2: API task burst
		// 3: Mixed workload
		switch cycle % 4 {
		case 0:
			// Background task burst
			tasksToCreate := rand.Intn(3) + 2
			log.Printf("Creating %d background tasks (cycle %d)", tasksToCreate, cycle)
			for range tasksToCreate {
				task := &BackgroundTask{
					TaskID: fmt.Sprintf("bg-%04d", taskID),
				}
				taskID++
				go func() {
					backgroundObserver.RunTask(task)
				}()
			}

		case 1:
			// Critical tasks
			tasksToCreate := rand.Intn(2) + 1
			log.Printf("Creating %d critical tasks (cycle %d)", tasksToCreate, cycle)
			for range tasksToCreate {
				task := &CriticalTask{
					TaskID: fmt.Sprintf("crit-%04d", taskID),
				}
				taskID++
				go func() {
					criticalObserver.RunTask(task)
				}()
			}

		case 2:
			// API task burst
			tasksToCreate := rand.Intn(8) + 5
			log.Printf("Creating %d API tasks (cycle %d)", tasksToCreate, cycle)
			for range tasksToCreate {
				task := &APITask{
					TaskID: fmt.Sprintf("api-%04d", taskID),
				}
				taskID++
				go func() {
					apiObserver.RunTask(task)
				}()
			}

		case 3:
			// Mixed workload
			bgTasks := rand.Intn(2) + 1
			critTasks := 1
			apiTasks := rand.Intn(4) + 2

			log.Printf("Mixed workload: %d background, %d critical, %d API tasks (cycle %d)",
				bgTasks, critTasks, apiTasks, cycle)

			// Background tasks
			for range bgTasks {
				task := &BackgroundTask{TaskID: fmt.Sprintf("bg-%04d", taskID)}
				taskID++
				go func() {
					backgroundObserver.RunTask(task)
				}()
			}

			// Critical tasks
			for range critTasks {
				task := &CriticalTask{TaskID: fmt.Sprintf("crit-%04d", taskID)}
				taskID++
				go func() {
					criticalObserver.RunTask(task)
				}()
			}

			// API tasks
			for range apiTasks {
				task := &APITask{TaskID: fmt.Sprintf("api-%04d", taskID)}
				taskID++
				go func() {
					apiObserver.RunTask(task)
				}()
			}
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
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("Error shutting down metrics server: %v", err)
		}
		os.Exit(0)
	}()

	log.Println("Multiple observers running... Press Ctrl+C to stop")
	log.Println("Metrics available at: http://localhost:8080/metrics")
	log.Println("Prometheus UI: http://localhost:9090")
	log.Println("Grafana UI: http://localhost:3000")

	run()
}
