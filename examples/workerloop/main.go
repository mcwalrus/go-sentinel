package main

import (
	"context"
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

// Job represents a unit of work
type MyJob struct {
	ID   int
	Data string
}

func (job *MyJob) Execute() error {
	log.Printf("Processing job %d: %s", job.ID, job.Data)
	processingTime := time.Duration(rand.Intn(3)+1) * 100 * time.Millisecond
	time.Sleep(processingTime)
	log.Printf("Completed job %d (took %v)", job.ID, processingTime)
	return nil
}

func run() {
	// Add some jobs to the pool
	jobs := []MyJob{
		{ID: 1, Data: "Process user registration"},
		{ID: 2, Data: "Generate report"},
		{ID: 3, Data: "Send email notifications"},
		{ID: 4, Data: "Backup database"},
		{ID: 5, Data: "Process payment"},
		{ID: 6, Data: "Update inventory"},
		{ID: 7, Data: "Clean temporary files"},
		{ID: 8, Data: "Sync with external API"},
	}

	log.Printf("Adding %d jobs to the queue", len(jobs))
	for _, job := range jobs {
		// Simulate long-running process (10 - 30 milliseconds)
		ob.RunFunc(func() error {
			log.Printf("Processing job %d: %s", job.ID, job.Data)
			processingTime := time.Duration(rand.Intn(3)+1) * 100 * time.Millisecond
			time.Sleep(processingTime)
			log.Printf("Completed job %d (took %v)", job.ID, processingTime)
			return nil
		})
	}

	// Let workers process for a bit
	time.Sleep(5 * time.Second)

	// Add a few more jobs to demonstrate continuous processing
	moreJobs := []MyJob{
		{ID: 9, Data: "Process refund"},
		{ID: 10, Data: "Update search index"},
	}

	log.Println("Adding additional jobs...")
	for _, job := range moreJobs {
		ob.RunFunc(func() error {
			log.Printf("Processing job %d: %s", job.ID, job.Data)
			processingTime := time.Duration(rand.Intn(3)+1) * 100 * time.Millisecond
			time.Sleep(processingTime)
			log.Printf("Completed job %d (took %v)", job.ID, processingTime)
			return nil
		})
	}

	// Run additional jobs periodically until shutdown
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Add a periodic job
	jobID := 11
	for range ticker.C {
		currentJobID := jobID
		jobID++
		ob.RunFunc(func() error {
			log.Printf("Processing periodic job %d", currentJobID)
			processingTime := time.Duration(rand.Intn(5)+1) * 200 * time.Millisecond
			time.Sleep(processingTime)
			log.Printf("Completed periodic job %d (took %v)", currentJobID, processingTime)
			return nil
		})
	}
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
