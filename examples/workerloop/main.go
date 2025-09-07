package main

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

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

	ob := NewObserver()

	log.Printf("Adding %d jobs to the queue", len(jobs))
	for _, job := range jobs {
		// Simulate long-running process (10 - 30 milliseconds)
		ob.ObserveFunc(func() error {
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
		ob.ObserveFunc(func() error {
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

type Config struct {
	Timeout    time.Duration
	MaxRetries int
	PromLabels map[string]string
}

func defaultConfig() Config {
	return Config{
		Timeout:    0,
		MaxRetries: 0,
		PromLabels: nil,
	}
}

type Task interface {
	Config() Config
	Execute() error
}

type Observer struct {
	cfg ObserverConfig
	wg  sync.WaitGroup
}

func NewObserver(cfg ObserverConfig) *Observer {
	return &Observer{
		cfg: cfg,
	}
}

type ObserverConfig struct {
	PromLabels map[string]string
}

func defaultObserverConfig() ObserverConfig {
	return ObserverConfig{
		PromLabels: make(map[string]string),
	}
}

func (o *Observer) Observe(task Task) {
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		fmt.Println("Observed task:", task.Execute())
	}()
}

func (o *Observer) ObserveFunc(fn func() error) {
	task := &implJob{
		fn:  fn,
		cfg: defaultConfig(),
	}
	o.Observe(task)
}

func (o *Observer) Wait() {
	o.wg.Wait()
}

type implJob struct {
	fn  func() error
	cfg Config
}

func (j *implJob) Config() Config {
	return j.cfg
}

func (j *implJob) Execute() error {
	return j.fn()
}
