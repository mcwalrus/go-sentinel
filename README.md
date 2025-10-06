# Sentinel

[![Go Version](https://img.shields.io/github/go-mod/go-version/mcwalrus/go-sentinel)](https://golang.org/)
[![Go Report Card](https://goreportcard.com/badge/github.com/mcwalrus/go-sentinel)](https://goreportcard.com/report/github.com/mcwalrus/go-sentinel)
[![codecov](https://codecov.io/gh/mcwalrus/go-sentinel/branch/main/graph/badge.svg)](https://codecov.io/gh/mcwalrus/go-sentinel) 
[![GoDoc](https://godoc.org/github.com/mcwalrus/go-sentinel?status.svg)](https://godoc.org/github.com/mcwalrus/go-sentinel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Sentinel provides retry handling and observability monitoring for Go applications. It wraps task execution with Prometheus metrics, observing for successes, errors caught, panic occurrences, retries, and timeouts - making critical routines more observable and resilient. Use the library as a drop-in solution for new projects or existing applications with ease.

## Features

- **Prometheus Metrics**: Observe tasks through pre-defined metrics
- **Composable Pattern**: Multiple observers can be employed at once
- **Retry Logic**: Enables retry strategies with circuit breaking support
- **Panic Recovery**: Panic recovery safety or standard panic propagation
- **Context Timeout**: Timeout support for handling task deadlines

## Metrics

Standard configuration will automatically export the following observer metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `sentinel_in_flight` | Gauge | Active number of running tasks |
| `sentinel_success_total` | Counter | Total successful task completions |
| `sentinel_errors_total` | Counter | Total task executions failures |
| `sentinel_timeouts_total` | Counter | Total timeout based failures |
| `sentinel_panics_total` | Counter | Total task panic occurrences |
| `sentinel_durations_seconds` | Histogram | Distribution of task executions |
| `sentinel_retries_total` | Counter | Total retry attempts after failures |


Note failed _retry attempts_ and _panic occurances_ are both counted as __errors_total__ counter.

## Installation

Library requires Go version >= 1.23:

```bash
go get github.com/mcwalrus/go-sentinel
```

## Usage Examples

### Basic Usage

Configure an observer and observe a task:

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
    // Create an observer
    observer := sentinel.NewObserver()

    // Register the observer
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

Observer records errors via metrics returning errors:

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
    // Create and register observer
    observer := sentinel.NewObserver()
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Observer records task errors
    for range 1000 {    
        err := observer.RunFunc(func() error {
            return errors.New("task failed")
        })
        if err != nil {
            log.Printf("Task failed: %v", err)
        }
    }
}
```

### Timeout Handling

Observer provides context timeouts based on TaskConfig:

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
    // Create and register observer
    observer := sentinel.NewObserver()
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Task config with timeout
    config := sentinel.TaskConfig{
        Timeout: 3 * time.Second,
    }

    // Respect context timeout
    err := observer.RunCtx(config, 
        func(ctx context.Context) error {
            <-ctx.Done()
            return ctx.Err()
        },
    )
    if !errors.Is(err, context.DeadlineExceeded) {
        panic("expected timeout error, got:", err)
    }
}
```

Timeouts are recorded by both `timeouts_total` and `errors_total` counters.

### Retry Handling

Configure retry with wait strategies for resilient task execution:

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
    // Create and register observer
    observer := sentinel.NewObserver()
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Exponential backoff retry strategy
    config := sentinel.TaskConfig{
        MaxRetries:    3,
        Timeout:       10 * time.Second,
        RetryStrategy: sentinel.RetryStrategyExponentialBackoff(100*time.Millisecond),
    }

    // Fail for two, pass next attempt
    var i int
    err := observer.Run(config, func() error {
        if i < 2 {
            i++
            return errors.New("no good, try again")
        } else {
            fmt.Println("it works!")
            return nil
        }
    })

    // Expect success on third attempt
    if err != nil {
        panic("unexpected error", err)
    }
}
```

Note a Task called with `MaxRetries=3` may be called up to four times total.

### Observe Durations

Histogram buckets need to be set by the Observer config to export `durations_seconds` metrics:

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
    // Create observer with histogram metrics:
    observer := sentinel.NewObserver(
        sentinel.WithHistogramBuckets([]float64{1, 5, 8, 12, 15}),
    )
    // Configure observer with registry
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Wait random intervals of time
    for range 1000 { 
        err := observer.Run(config, func() error {
            sleep := time.Duration(rand.Intn(20)+1) * time.Second
            fmt.Printf("Sleeping for %v...\n", sleep)
            time.Sleep(sleep)
            return nil
        })
    }
    // Expect no error
    if err != nil {
        panic("unexpected error", err)
    }
}
```

### Panic Occurances

Panic occurances are just returned as errors:

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
    // Create and register observer
    observer := sentinel.NewObserver()
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)
    
    // Panic multiple times
    config := sentinel.TaskConfig{MaxRetries: 3}
    err := observer.Run(config, func() error {
        panic("panic stations!")
    })
    
    // Unwrap errors
    errUnwrap, ok := (err).(interface {Unwrap() []error})
    if !ok {
        panic("not unwrap")
    }

    // Panics are errors
    errs := errUnwrap.Unwrap()
    for _, err := range errs {
        // View panic recovery values
        var errPanic = &sentinel.ErrPanicOccurred{}
        if errors.As(err, &errPanic) {
            fmt.Printf("panic value: %v\n", errPanic.PanicValue())
        }
    }
}
```

Panics are always recorded with `panics_total` and `errors_total` counters.

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

    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // Create observer
    observer := sentinel.NewObserver(
	    sentinel.WithNamespace("myapp"),
	    sentinel.WithSubsystem("workers"),
    )

    // Register observer
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
    
    // Your application code ...
    for range time.NewTicker(3 * time.Second).C {
        err := observer.RunFunc(doFunc)
        if err != nil {
            fmt.Formatf("error occurred: %w\n", err)
        }
    }
}
```

Exposed metrics will be presented with fully qualified names `myapp_workers_...` on host: _localhost:8080/metrics_.

## Contributing

Please report any issues or feature requests to the [GitHub repository](https://github.com/mcwalrus/go-sentinel).

## About

This module is maintained by Max Collier under an MIT License Agreement.