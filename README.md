# Sentinel

[![Go Version](https://img.shields.io/github/go-mod/go-version/mcwalrus/go-sentinel)](https://golang.org/)
[![Go Report Card](https://goreportcard.com/badge/github.com/mcwalrus/go-sentinel)](https://goreportcard.com/report/github.com/mcwalrus/go-sentinel)
[![codecov](https://codecov.io/gh/mcwalrus/go-sentinel/branch/main/graph/badge.svg)](https://codecov.io/gh/mcwalrus/go-sentinel) 
[![GoDoc](https://godoc.org/github.com/mcwalrus/go-sentinel?status.svg)](https://godoc.org/github.com/mcwalrus/go-sentinel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Sentinel provides resilience for Go applications with automatic retry handling and Prometheus observability. Considering that **panics should be treated as errors in production systems for critical processes**, sentinel wraps function execution to track successes, errors, panics, retries, timeouts and durations - making critical your routines more resilient, observable and robust. Use the library as a simple drop-in solution for new projects or existing applications.

## Features

- **Prometheus Metrics**: Observe tasks through pre-defined metrics
- **Composable Pattern**: Multiple observers can be employed at once
- **Retry Logic**: Enables retry strategies with circuit breaking support
- **Panic Recovery**: Safe panic recovery or standard panic propagation
- **Context Timeout**: Timeout support for handling task deadlines

## Metrics

Standard configuration will automatically export the following observer metrics:

| Metric | Type | Description | Default | Option |
|--------|------|-------------|---------|--------|
| `sentinel_in_flight` | Gauge | Active number of running tasks | Yes | - |
| `sentinel_successes_total` | Counter | Total successful tasks | Yes | - |
| `sentinel_failures_total` | Counter | Total failed tasks | Yes | - |
| `sentinel_errors_total` | Counter | Total errors over all attempts | Yes | - |
| `sentinel_panics_total` | Counter | Total panic occurrences | Yes | - |
| `sentinel_durations_seconds` | Histogram | Task execution durations in buckets | No | _WithDurationMetrics_ |
| `sentinel_timeouts_total` | Counter | Total errors based on timeouts | No | _WithTimeoutMetrics_ |
| `sentinel_retries_total` | Counter | Total retry attempts for tasks | No | _WithRetryMetrics_ |

You can configure exported observer metrics based on your application needs.

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
)

func main() {
    // Create new observer
    observer := sentinel.NewObserver()
    
    // Execute your task
    err := observer.Run(func() error {
        return nil
    })
    // Handle task error
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
)

func main() {
    // Create new observer
    observer := sentinel.NewObserver()
    
    // Counts error
    err := observer.Run(func() error {
        return errors.New("task failed")
    })    
    // Handle task error
    if err != nil {
        log.Printf("Task failed: %v", err)
    }
}
```

### Timeout Handling

Observer provides context timeouts based on ObserverConfig:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
)

func main() {
    // Observer tracks timeout metrics
    observer := sentinel.NewObserver(
        sentinel.WithTimeoutMetrics(),
    )

    // Set timeout for observer tasks
    observer.UseConfig(sentinel.ObserverConfig{
        Timeout: 10 * time.Second,
    })

    // Method respects context timeout
    err := observer.RunFunc(func(ctx context.Context) error {
            <-ctx.Done()
            return ctx.Err()
        },
    )
    if !errors.Is(err, context.DeadlineExceeded) {
        panic("expected timeout error, got:", err)
    }
}
```

Timeout errors are recorded by both `timeouts_total` and `errors_total` counters.

Note, timeout metrics will not be exported unless `WithTimeoutMetrics()` is set.

### Panic Handling

Panic occurrences are just returned as errors by the observer:

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
    // Create new observer
    observer := sentinel.NewObserver()
    
    // Panic multiple times
    err := observer.Run(func() error {
        panic("panic stations!")
    })
    // Handle task error
    if err != nil {
        log.Printf("Task failed: %v", err)
    }
    // Recover panic value
    if rPanic, ok := sentinel.IsPanicError(err); ok {
        fmt.Printf("panic value: %v\n", rPanic)
    }
}
```

Panics are always recorded with `panics_total` and `errors_total` counters. 

Panics can be allowed to propogate from the observer with: `DisableRecovery(true)`.

### Observe Durations

Set histogram buckets with the observer to export `durations_seconds` metrics:

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "math/rand"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
)

func main() {
    // Observer with duration metrics
    observer := sentinel.NewObserver(
        sentinel.WithDurationMetrics([]float64{
            0.100, 0.250, 0.400, 0.500, 1.000, // in seconds
        }),
    )
    
    // Run many times ...
    for i := 0; i < 100; i++ {

        // Tasks run between 50-500ms before return
        err := observer.RunFunc(func(ctx context.Context) error {
            sleep := time.Duration(rand.Intn(450)+50) * time.Millisecond
            fmt.Printf("Sleeping for %v...\n", sleep)
            time.Sleep(sleep)
            return nil
        })
        // Handle task error
        if err != nil {
            log.Printf("Task failed: %v", err)
        }
    }
}
```

Timeouts are always recorded with `timeouts_total` and `errors_total` counters.

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
    "github.com/mcwalrus/go-sentinel/retry"
)

func main() {
    // Observer with retry metrics
    observer := sentinel.NewObserver(
        sentinel.WithRetryMetrics(),           
    )

    // Observer retry configuration
    observer.UseConfig(sentinel.ObserverConfig{
        MaxRetries:    3,
        RetryStrategy: retry.WithJitter(
            retry.Exponential(100*time.Millisecond),
            time.Second,
        ),
    })

    // Error on every attempt
    err := observer.Run(func() error {
        return errors.New("task failed")
    })
    
    // Unwrap errors.Join
    errUnwrap, ok := (err).(interface {Unwrap() []error})
    if !ok {
        panic("not unwrap")
    }

    // Handle all errors
    errs := errUnwrap.Unwrap()
    for i, err := range errs {
        log.Printf("Task failed: %d: %v", i, err)
    }
}
```

Tasks called with `MaxRetries=3` may be called up to _four times_ total.

### Prometheus Integration

Use template for integrating sentinel with a prometheus endpoint:

```go
import (
    "fmt"
    "time"
    "net/http"

    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // Create new observer
    observer := sentinel.NewObserver(
	    sentinel.WithNamespace("myapp"),
	    sentinel.WithSubsystem("workers"),
    )
    
    // Register with registry
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
    
    // Your application code
    for range time.NewTicker(3 * time.Second).C {
        err := observer.RunFunc(doFunc)
        if err != nil {
            fmt.Printf("error occurred: %v\n", err)
        }
    }
}
```

Prometheus metrics will be exposed with names `myapp_workers_...` on host _localhost:8080/metrics_.

### Using Multiple Observers

Configure multiple observers for different task types and create forks for specialised behaviour:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    "time"

    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // Create first observer
    bgObserver := sentinel.NewObserver(
        sentinel.WithSubsystem("background"),
    )
    bgObserver.UseConfig(sentinel.ObserverConfig{
        Timeout:    30 * time.Second,
        MaxRetries: 1,
    })

    // Create a second observer
    critObserver := sentinel.NewObserver(
        sentinel.WithSubsystem("critical"),
    )
    critObserver.UseConfig(sentinel.ObserverConfig{
        Timeout:    60 * time.Second,
        MaxRetries: 2,
    })

    // Create fork of second observer
    critObserver2 := critObserver.Fork()
    critObserver2.UseConfig(sentinel.ObserverConfig{
        Timeout:    120 * time.Second,
        MaxRetries: 4,
    })

    // Only register root observers
    registry := prometheus.NewRegistry()
    bgObserver.MustRegister(registry)
    critObserver.MustRegister(registry)

    // Set up metrics endpoint
    http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
    go func() {
        log.Println("Metrics server starting on :8080/metrics")
        log.Fatal(http.ListenAndServe(":8080", nil))
    }()

    // Run forever
    select {}
}
```

The result of this will be that we have two sets of observer metrics `background_...` and `critical_...`. 

The API observers share the underlying exposed metrics where both observers run with their own configurations.

## Contributing

Please report any issues or feature requests to the [GitHub repository](https://github.com/mcwalrus/go-sentinel).

I am particularly keen to hear feedback around how to appropriately present the library alongside issues.

## About

This module is maintained by Max Collier under an MIT License Agreement.