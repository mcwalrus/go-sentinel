# Sentinel

Sentinel provides reliability with observability monitoring for tasks in Go applications. It wraps task execution with Prometheus metrics, error handling, retries, and timeouts — making critical routines safe, measurable, and reliable. Use the library can be used as a drop-in solution for new or existing applications. 

## Features

- **Prometheus Metrics**: Comprehensive observability with built-in metrics
- **Error Handling**: Graceful error handling with configurable failure strategies  
- **Retry Logic**: Configurable retry strategies (immediate, linear backoff, exponential backoff)
- **Panic Recovery**: Safe panic recovery with optional propagation
- **Timeouts**: Context-based timeout handling
- **Concurrency Control**: Synchronous or asynchronous task execution

## Metrics

Sentinel automatically exports the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `sentinel_in_flight` | Gauge | Current number of running tasks |
| `sentinel_success_total` | Counter | Total successful task completions |
| `sentinel_errors_total` | Counter | Total failed tasks |
| `sentinel_timeout_total` | Counter | Total timed-out tasks |
| `sentinel_panics_total` | Counter | Total panics recovered in tasks |
| `sentinel_runtime_seconds` | Histogram | Task execution duration distribution |
| `sentinel_retries_total` | Counter | Total retry attempts |

## Installation

Library requires Go version >= 1.23:

```bash
go get github.com/mcwalrus/go-sentinel
```

## Usage Examples

### Basic Usage

Simple task execution with default configuration:

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    // Create observer with default configuration
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    
    // Register with Prometheus
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Execute a simple task
    err := observer.RunFunc(func() error {
        fmt.Println("Processing task...")
        // Your task logic here
        return nil
    })
    
    if err != nil {
        log.Printf("Task failed: %v", err)
    }
}
```

### Multiple Observers

Using multiple observers with different configurations for different types of tasks:

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    registry := prometheus.NewRegistry()
    
    // Observer for critical tasks with fine-grained metrics
    observer1 := sentinel.NewObserver(sentinel.ObserverConfig{
        Namespace:   "app",
        Subsystem:   "background_tasks",
        Description: "Background processing tasks",
        Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30},
    })
    observer1.MustRegister(registry)
    
    // Observer for background jobs with coarser metrics
    observer2 := sentinel.NewObserver(sentinel.ObserverConfig{
        Namespace:   "app", 
        Subsystem:   "critical_tasks",
        Description: "Critical business operations",
        Buckets:     []float64{1, 10, 60, 300, 1800}, // Longer running tasks
    })
    observer2.MustRegister(registry)
    
    // Background task executed with Concurrent=true
    _ = observer1.Run(sentinel.TaskConfig{
        Timeout:       5 * time.Minute,
        Concurrent:    true,
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        fmt.Println("Processing background task...")
        time.Sleep(30 * time.Second)
        return nil
    })

    // Critical task is blocking with Concurrent=false
    _ = observer2.Run(sentinel.TaskConfig{
        Timeout:       30 * time.Second,
        Concurrent:    false,
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        fmt.Println("Critical business operations...")
        time.Sleep(2 * time.Second)
        return nil
    })

    fmt.Println("Waits for critial task to complete")
}
```

### Failure Handlers

Handling different types of failures and errors:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "math/rand"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Task that may fail
    err := observer.Run(sentinel.TaskConfig{
        Timeout:       10 * time.Second,
        Concurrent:    false,
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        // Simulate different failure scenarios
        switch rand.Intn(4) {
        case 0:
            return nil // Success
        case 1:
            return errors.New("business logic error")
        case 2:
            return fmt.Errorf("validation failed: invalid input")
        case 3:
            // Simulate timeout by sleeping longer than context timeout
            select {
            case <-time.After(15 * time.Second):
                return nil
            case <-ctx.Done():
                return ctx.Err() // Will be context.DeadlineExceeded
            }
        }
        return nil
    })
    
    if err != nil {
        fmt.Printf("Task failed with error: %v\n", err)
    }
    
    // Custom task implementation with error handling
    type DatabaseTask struct {
        Query string
        Params []interface{}
    }
    
    dbTask := &DatabaseTask{
        Query: "SELECT * FROM users WHERE id = ?",
        Params: []interface{}{123},
    }
    
    observer.RunTask(&TaskWithErrorHandling{task: dbTask})
}

type TaskWithErrorHandling struct {
    task *DatabaseTask
}

func (t *TaskWithErrorHandling) Config() sentinel.TaskConfig {
    return sentinel.TaskConfig{
        Timeout:       30 * time.Second,
        Concurrent:    false,
        RecoverPanics: true,
        MaxRetries:    2,
        RetryStrategy: sentinel.RetryStrategyExponentialBackoff(1 * time.Second),
    }
}

func (t *TaskWithErrorHandling) Execute(ctx context.Context) error {
    fmt.Printf("Executing query: %s\n", t.task.Query)
    
    // Simulate database operations that might fail
    if rand.Float64() < 0.3 {
        return errors.New("database connection failed")
    }
    
    // Check for context cancellation
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        fmt.Println("Query executed successfully")
        return nil
    }
}
```

### Retry Handling

Configuring different retry strategies for resilient task execution:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "math/rand"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Immediate retry - retry without delay
    fmt.Println("=== Immediate Retry Strategy ===")
    observer.Run(sentinel.TaskConfig{
        MaxRetries:    3,
        RetryStrategy: sentinel.RetryStrategyImmediate,
        Concurrent:    false,
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        if rand.Float64() < 0.7 { // 70% failure rate
            return errors.New("temporary service unavailable")
        }
        fmt.Println("Task succeeded!")
        return nil
    })
    
    time.Sleep(1 * time.Second)
    
    // Linear backoff - increasing delay linearly
    fmt.Println("=== Linear Backoff Strategy ===")
    observer.Run(sentinel.TaskConfig{
        MaxRetries:    4,
        RetryStrategy: sentinel.RetryStrategyLinearBackoff(500 * time.Millisecond),
        Concurrent:    false,
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        if rand.Float64() < 0.6 { // 60% failure rate
            fmt.Println("API call failed, will retry with linear backoff...")
            return errors.New("API rate limit exceeded")
        }
        fmt.Println("API call succeeded!")
        return nil
    })
    
    time.Sleep(1 * time.Second)
    
    // Exponential backoff - exponentially increasing delay
    fmt.Println("=== Exponential Backoff Strategy ===")
    observer.Run(sentinel.TaskConfig{
        MaxRetries:    5,
        RetryStrategy: sentinel.RetryStrategyExponentialBackoff(200 * time.Millisecond),
        Timeout:       30 * time.Second,
        Concurrent:    false,
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        if rand.Float64() < 0.8 { // 80% failure rate
            fmt.Println("Network request failed, will retry with exponential backoff...")
            return errors.New("network timeout")
        }
        fmt.Println("Network request succeeded!")
        return nil
    })
    
    // Custom retry strategy
    customRetryStrategy := func(retryCount int) time.Duration {
        // Custom logic: cap at 5 seconds max delay
        delay := time.Duration(retryCount*retryCount) * time.Second
        if delay > 5*time.Second {
            delay = 5 * time.Second
        }
        return delay
    }
    
    fmt.Println("=== Custom Retry Strategy ===")
    observer.Run(sentinel.TaskConfig{
        MaxRetries:    3,
        RetryStrategy: customRetryStrategy,
        Concurrent:    false,
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        if rand.Float64() < 0.5 { // 50% failure rate
            return errors.New("custom retry scenario")
        }
        fmt.Println("Custom retry task succeeded!")
        return nil
    })
}
```

### Panic Recovery

Handling panics gracefully with recovery options:

```go
package main

import (
    "context"
    "fmt"
    "math/rand"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Task with panic recovery enabled (default behavior)
    fmt.Println("=== Panic Recovery Enabled ===")
    err := observer.Run(sentinel.TaskConfig{
        RecoverPanics: true,  // Panics will be recovered and counted
        Concurrent:    false,
        MaxRetries:    2,
        RetryStrategy: sentinel.RetryStrategyLinearBackoff(1 * time.Second),
    }, func(ctx context.Context) error {
        fmt.Println("About to panic...")
        panic("something went terribly wrong!")
    })
    
    fmt.Printf("Task completed, error: %v\n", err)
    fmt.Println("Application continues running after panic recovery")
    
    time.Sleep(1 * time.Second)
    
    // Task with conditional panic
    fmt.Println("=== Conditional Panic Scenario ===")
    observer.Run(sentinel.TaskConfig{
        RecoverPanics: true,
        Concurrent:    true, // Run concurrently to not block
        MaxRetries:    1,
    }, func(ctx context.Context) error {
        taskID := rand.Intn(1000)
        fmt.Printf("Processing task %d...\n", taskID)
        
        // Simulate different outcomes
        switch rand.Intn(4) {
        case 0:
            // Normal success
            fmt.Printf("Task %d completed successfully\n", taskID)
            return nil
        case 1:
            // Regular error
            return fmt.Errorf("task %d failed with business logic error", taskID)
        case 2:
            // Panic scenario
            panic(fmt.Sprintf("unexpected panic in task %d", taskID))
        case 3:
            // Nil pointer dereference (common panic cause)
            var ptr *string
            fmt.Println(*ptr) // This will panic
            return nil
        }
        return nil
    })
    
    time.Sleep(2 * time.Second)
    
    // Multiple concurrent tasks with potential panics
    fmt.Println("=== Multiple Concurrent Tasks with Panics ===")
    for i := 0; i < 5; i++ {
        taskNum := i
        observer.Run(sentinel.TaskConfig{
            RecoverPanics: true,
            Concurrent:    true,
            Timeout:       5 * time.Second,
        }, func(ctx context.Context) error {
            fmt.Printf("Concurrent task %d starting...\n", taskNum)
            
            // Simulate work
            time.Sleep(time.Duration(rand.Intn(1000)) * time.Millisecond)
            
            // Some tasks will panic, others will succeed
            if rand.Float64() < 0.3 { // 30% panic rate
                panic(fmt.Sprintf("panic in concurrent task %d", taskNum))
            }
            
            fmt.Printf("Concurrent task %d completed successfully\n", taskNum)
            return nil
        })
    }
    
    // Wait for concurrent tasks to complete
    time.Sleep(3 * time.Second)
    
    // Example of panic that would NOT be recovered (RecoverPanics: false)
    // Uncomment to see the difference - this would crash the program
    /*
    fmt.Println("=== Panic Recovery Disabled (Dangerous!) ===")
    observer.Run(sentinel.TaskConfig{
        RecoverPanics: false, // Panic will crash the program
        Concurrent:    false,
    }, func(ctx context.Context) error {
        panic("this will crash the program!")
    })
    */
    
    fmt.Println("All panic recovery examples completed successfully!")
}
```

## Advanced Usage

### Custom Task Implementation

```go

// Your task
type EmailTask struct {
    To      string
    Subject string
    Body    string
}

// Define sentinel configuration
func (e *EmailTask) Config() sentinel.TaskConfig {
    return sentinel.TaskConfig{
        Timeout:       30 * time.Second,
        MaxRetries:    3,
        RetryStrategy: sentinel.RetryStrategyExponentialBackoff(1 * time.Second),
        RecoverPanics: true,
        Concurrent:    true,
    }
}

// Implement sentinel task handler
func (e *EmailTask) Execute(ctx context.Context) error {
    fmt.Printf("Sending email to %s: %s\n", e.To, e.Subject)
    
    // Your email sending logic here
    return nil
}

// Example usage:
observer.RunTask(&EmailTask{
    To:      "user@example.com",
    Subject: "Welcome!",
    Body:    "Welcome to our service!",
})
```

### Prometheus Integration

Use template for integrating sentinel with a prometheus endpoint:

```go
import (
    "fmt"
    "net/http"

    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    observer := sentinel.NewObserver(sentinel.ObserverConfig{
        Namespace: "myapp",
        Subsystem: "workers",
    })
    
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Expose metrics endpoint
    http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
    go func() {
        err := http.ListenAndServe(":8080", nil)
        if err != nil {
            fmt.Fatal(err)
        }
    }()
    
    // Your application code...
}
```

## Roadmap

- [ ] Labels support, Vec support
- [ ] Circuit breaker with supported Prometheus metrics
- [ ] Distributed tracing integration
- [ ] Rate limiting capabilities
