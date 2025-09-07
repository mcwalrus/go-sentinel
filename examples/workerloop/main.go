package main

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	viewprom "github.com/mcwalrus/view-prom"
)

var ob *viewprom.Observer

func init() {
	ob = viewprom.NewObserver(viewprom.ObserverConfig{
		Namespace:   "example",
		Subsystem:   "workerloop",
		Description: "Worker loop",
		Buckets:     []float64{0.01, 0.1, 1, 10, 100, 1000, 10_000},
	})
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

func main() {

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
		ob.Observe(func() error {
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
		ob.Observe(func() error {
			log.Printf("Processing job %d: %s", job.ID, job.Data)
			processingTime := time.Duration(rand.Intn(3)+1) * 100 * time.Millisecond
			time.Sleep(processingTime)
			log.Printf("Completed job %d (took %v)", job.ID, processingTime)
			return nil
		})
	}

	// Wait a bit more then shutdown
	time.Sleep(8 * time.Second)

	fmt.Println("Application finished")
}
