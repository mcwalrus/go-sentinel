# Sentinel

Sentinel provides reliability handling and observability monitoring in Go applications. It wraps task execution with Prometheus metrics, observing errors, panic occurances, retries, and timeouts — making critical routines safe, measurable, and reliable. Use the library as a drop-in solution for new projects or existing applications.

## Features

- **Prometheus Metrics**: Comprehensive observability with built-in metrics
- **Timeouts**: Context-based timeout handling for processes
- **Errors**: Observed errors are reported by metrics 
- **Panic Recovery**: Safe panic recovery with optional propagation
- **Concurrency Control**: Synchronous or asynchronous task execution
- **Retry Logic**: Configurable retry strategies

## Metrics

Default configuration automatically exports the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `sentinel_in_flight` | Gauge | Current number of running tasks |
| `sentinel_successes_total` | Counter | Total successful task completions |
| `sentinel_errors_total` | Counter | Total task execution failures |
| `sentinel_timeout_errors_total` | Counter | Total timed-out based failures |
| `sentinel_panic_occurances_total` | Counter | Total panic occurances in tasks |
| `sentinel_observed_duration_seconds` | Histogram | Task execution duration distribution |
| `sentinel_retry_attempts_total` | Counter | Total retry attempts after failures |

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
    // Handle your task error
    if err != nil {
        log.Printf("Task failed: %v", err)
    }
}
```

### Failure Handlers

Handle task failures with error logging:

```go
package main

import (
    "errors"
    "fmt"
    "log"
    "math/rand"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Run many times
    for i := 0; i < 1000; i++ {
        
        // Task errors are recorded by the observer
        err := observer.RunFunc(func() error {
            if rand.Float64() < 0.5 {
                return errors.New("your task failed")
            }
            fmt.Println("task was successful!")
            return nil
        })

        // Provide custom error handling per error
        if err != nil {
            log.Printf("Task failed: %v", err)
        }
    }
}
```

### Retry Handling

Configure retry strategies for resilient task execution:

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
    
    // Exponential backoff retry strategy
    config := sentinel.TaskConfig{
        MaxRetries:    3,
        Timeout:       10 * time.Second,
        RetryStrategy: sentinel.RetryStrategyExponentialBackoff(100 * time.Millisecond),
    }

    // Retry failures, succeed after two attempts
    var i int
    err := observer.Run(config, func(ctx context.Context) error {
        if i < 2 {
            i++
            fmt.Println("failed :( will retry...")
            return errors.New("no good, try again")
        }
        fmt.Println("it works!")
        return nil
    })

    // Expect run to succeed on final retry attempt
    if err != nil {
        panic("unexpected error", err)
    }
}
```

### Panic Recovery

Handle panics gracefully with recovery options:

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Panic recovery will be recovered and counted in metrics
    config := sentinel.TaskConfig{
        RecoverPanics: true,
        Concurrent:    false,
    }
    observer.Run(config, func(ctx context.Context) error {
        fmt.Println("Oh no, panic...")
        panic("something went wrong!")
    })
    fmt.Println("continues after panic recovery")
    
    // Panic recovery disabled can allow the program to crash
    func() {
        config := sentinel.TaskConfig{
            RecoverPanics: false,
            Concurrent:    false,
        }

        defer func() {
            if r := recover(); r != nil {
                fmt.Println("r,r,r- recovered!")
            } 
        }()

        observer.Run(config, func(ctx context.Context) error {
            fmt.Println("Oh no, I've slipped...")
            panic("this will crash the program!")
        })
    }()

    fmt.Println("Oh, you found me! :0")
}
```

### Task Implementation

An alternative means to describe tasks through `sentinel.Task` interface:

```go
// Example task
type EmailTask struct {
    To      string
    Subject string
    Body    string
}

// Define task configuration
func (e *EmailTask) Config() sentinel.TaskConfig {
    return sentinel.TaskConfig{
        Timeout:       30 * time.Second,
        MaxRetries:    3,
        Concurrent:    true,
        RecoverPanics: true,
        RetryStrategy: sentinel.RetryStrategyExponentialBackoff(1 * time.Second),
    }
}

// Implement task handling
func (e *EmailTask) Execute(ctx context.Context) error {
    fmt.Printf("Sending email to %s: %s\n", e.To, e.Subject)
    return nil
}

// Example usage
observer.RunTask(&EmailTask{
    To:      "user@example.com",
    Subject: "Welcome!",
    Body:    "Welcome to our service!",
})
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
    
    // First observer
    observer1 := sentinel.NewObserver(sentinel.ObserverConfig{
        Namespace:   "app",
        Subsystem:   "background_tasks",
        Description: "Background processing tasks",
        Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 30},
    })
    observer1.MustRegister(registry)
    
    // Second observer
    observer2 := sentinel.NewObserver(sentinel.ObserverConfig{
        Namespace:   "app", 
        Subsystem:   "critical_tasks",
        Description: "Critical business operations",
        Buckets:     []float64{1, 10, 60, 300, 1800},
    })
    observer2.MustRegister(registry)
    
    _ = observer1.Run(sentinel.TaskConfig{
        Timeout:       5 * time.Minute,
        Concurrent:    true, // Background tasks
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        fmt.Println("Processing background task...")
        time.Sleep(30 * time.Second)
        return nil
    })

    _ = observer2.Run(sentinel.TaskConfig{
        Timeout:       30 * time.Second,
        Concurrent:    false, // Synchronous task
        RecoverPanics: true,
    }, func(ctx context.Context) error {
        fmt.Println("Critical business operations...")
        time.Sleep(2 * time.Second)
        return nil
    })

    // Waits for blocking task execution ...
    fmt.Println("Waits for critial task to complete")
}
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

- [ ] Consider labels support, Vec support
- [ ] Circuit breaker with exposed Prometheus metrics
- [ ] Distributed tracing integration, otel

## Contributing

Please report any issues or feature requests to the [GitHub repository](https://github.com/mcwalrus/go-jitjson).

## About

This module is maintained by Max Collier under an MIT License Agreement.