# Sentinel

[![Go Version](https://img.shields.io/github/go-mod/go-version/mcwalrus/go-sentinel)](https://golang.org/)
[![Go Report Card](https://goreportcard.com/badge/github.com/mcwalrus/go-sentinel)](https://goreportcard.com/report/github.com/mcwalrus/go-sentinel)
[![codecov](https://codecov.io/gh/mcwalrus/go-sentinel/branch/main/graph/badge.svg)](https://codecov.io/gh/mcwalrus/go-sentinel) 
[![GoDoc](https://godoc.org/github.com/mcwalrus/go-sentinel?status.svg)](https://godoc.org/github.com/mcwalrus/go-sentinel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Sentinel provides reliability handling and observability monitoring in Go applications. It wraps task execution with Prometheus metrics, observing for successes, errors caught, panic occurrences, retries, and timeouts - making critical routines more resilient, observable, and robust. Use the library as a drop-in solution for new projects or existing applications.

## Features

- **Prometheus Metrics**: Observe tasks from pre-defined metrics
- **Composable Pattern**: Multiple observers can be employed at once
- **Retry Logic**: Enables retry strategies with circuit breaking support
- **Panic Recovery**: Panic recovery safety or standard panic propagation
- **Context Timeout**: Timeout support for handling task deadlines

## Metrics

Default configuration automatically exports the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `sentinel_in_flight` | Gauge | Active number of running tasks |
| `sentinel_success_total` | Counter | Total successful task completions |
| `sentinel_errors_total` | Counter | Total task executions failures |
| `sentinel_timeouts_total` | Counter | Total timeout based failures |
| `sentinel_panics_total` | Counter | Total task panic occurrences |
| `sentinel_durations_seconds` | Histogram | Distribution of task executions |
| `sentinel_retries_total` | Counter | Total retry attempts after failures |


Note failed retry attempts are counted with the __errors_total__ counter.

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
    // Observer with default configuration
    observer := sentinel.NewObserver(sentinel.DefaultConfig())

    // Register observer to a registry
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Execute a simple task
    err := observer.RunFunc(func() error {
        fmt.Println("Processing task...")
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
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Run many times
    for i := 0; i < 1000; i++ {
        
        // Observer records task errors
        err := observer.RunFunc(func() error {
            return errors.New("your task failed")
        })
        // Provide custom error handling per error
        if err != nil {
            log.Printf("Task failed: %v", err)
        }
    }
}
```

### Timeout Handling

Observer registers timeout errors for both `timeouts_total` and `errors_total` counters:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    observer := sentinel.NewObserver(sentinel.DefaultConfig())
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // TaskConfig provides configurable Timeout
    config := sentinel.TaskConfig{
        Timeout: 3 * time.Second,
    }

    // Task waits timeout duration respects context timeout
    err := observer.Run(config, func(ctx context.Context) error {
        <-ctx.Done()
        return ctx.Err()
    })
    if !errors.Is(err, context.DeadlineExceeded) {
        panic("expected timeout error, got:", err)
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
    }
    err := observer.Run(config, func(ctx context.Context) error {
        fmt.Println("Oh no, panic...")
        panic("something went wrong!")
    })
    
    // An error is returned after recovery showing panic occurred
    if err != nil {
        fmt.Printf("Task completed with recovered panic: %v\n", err)
    } else {
        panic("expected error, but got nil")
    }

    fmt.Println("continues after panic recovery")
}
```

Note panic occurrences are counted even when `RecoverPanics=false`. Enabling panic recovery should be explicitly set through TaskConfig in case panic propagation should signal specific handling up the stack, or other actions to be taken by the application. `RecoverPanics=true` when tasks are observed through the Observer method [RunFunc](https://pkg.go.dev/github.com/mcwalrus/go-sentinel#Observer.RunFunc).

### Concurrent Mode

Concurrent tasks can be executed and observed through the observer:

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
    
    // Concurrency task perform through go-routinue
    config := sentinel.TaskConfig{
        Concurrent:    true,
    }
    // Errors from concurrent tasks are not returned
    err := observer.Run(config, func(ctx context.Context) error {
        return fmt.Errorf("error occurred")
    })
    if err != nil {
        // It would be ok to ignore the error instead
        panic("no error expected, got", err)
    }
}
```

Note the error will be counted through the `errors_total` counters.

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

## Contributing

Please report any issues or feature requests to the [GitHub repository](https://github.com/mcwalrus/go-sentinel).

## About

This module is maintained by Max Collier under an MIT License Agreement.