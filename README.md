# Sentinel

[![Go Version](https://img.shields.io/github/go-mod/go-version/mcwalrus/go-sentinel)](https://golang.org/)
[![Go Report Card](https://goreportcard.com/badge/github.com/mcwalrus/go-sentinel)](https://goreportcard.com/report/github.com/mcwalrus/go-sentinel)
[![codecov](https://codecov.io/gh/mcwalrus/go-sentinel/branch/main/graph/badge.svg)](https://codecov.io/gh/mcwalrus/go-sentinel) 
[![GoDoc](https://godoc.org/github.com/mcwalrus/go-sentinel?status.svg)](https://godoc.org/github.com/mcwalrus/go-sentinel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Sentinel provides retry handling and observability monitoring for Go applications. It wraps functions or task execution with Prometheus metrics, observing for successes, errors caught, panic occurrences, retries, and timeouts - making critical routines more resilient, observable and robust. A core principle of design is that **panics should be treated as errors in production systems**. Use the library as a drop-in solution for new projects or existing applications.


## Features

- **Prometheus Metrics**: Observe tasks through pre-defined metrics
- **Composable Pattern**: Multiple observers can be employed at once
- **Retry Logic**: Enables retry strategies with circuit breaking support
- **Panic Recovery**: Panic recovery safety or standard panic propagation
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
    
    // Execute simple task
    err := observer.Run(func() error {
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
)

func main() {
    // Create new observer
    observer := sentinel.NewObserver()
    
    // Records task errors
    err := observer.Run(func() error {
        return errors.New("task failed")
    })    
    // Handle your task error
    if err != nil {
        log.Printf("Task failed: %v", err)
    }
}
```

### Timeout Handling

Observer provides context timeouts based on Config:

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
    // Observer with timeout metrics
    observer := sentinel.NewObserver(
        sentinel.WithTimeoutMetrics(),
    )

    // Set timeout via UseConfig
    observer.UseConfig(sentinel.Config{
        Timeout: 10 * time.Second,
    })

    // Respects context timeout
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

When timeout metrics are enabled, timeouts are recorded by both `timeouts_total` and `errors_total` counters.

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

    // Set retry configuration
    observer.UseConfig(sentinel.Config{
        MaxRetries:    3,
        Timeout:       10 * time.Second,
        RetryStrategy: retry.WithJitter(
            retry.Exponential(100*time.Millisecond),
            time.Second,
        ),
    })

    // Fail twice, pass on third attempt
    var i int
    err := observer.Run(func() error {
        if i++; i < 2 {
            return errors.New("no good, try again")
        } else {
            fmt.Println("it works!")
            return nil
        }
    })
    // Handle your task error
    if err != nil {
        fmt.Printf("Task failed after all retries: %v\n", err)
    }
}
```

Errors are always recorded with `panics_total` and `errors_total` counters. Note a task called with `MaxRetries=3` may be called up to _four times_ total.

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
    // Observer with histogram buckets
    observer := sentinel.NewObserver(
        sentinel.WithDurationMetrics([]float64{1, 5, 8, 12, 15}),
    )
    
    // Wait random intervals of time
    for range 1000 { 
        err := observer.RunFunc(func(ctx context.Context) error {
            sleep := time.Duration(rand.Intn(20)+1) * time.Second
            fmt.Printf("Sleeping for %v...\n", sleep)
            time.Sleep(sleep)
            return nil
        })
        // Expect no error
        if err != nil {
            panic("unexpected error", err)
        }
    }
}
```

Note, duration metrics will not be exported unless `WithDurationMetrics()` is set.

### Panic Occurrences

Panic occurrences are just returned as errors:

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
    observer.UseConfig(sentinel.Config{MaxRetries: 3})
    err := observer.Run(func() error {
        panic("panic stations :0")
    })
    
    // Unwraps errors.Join()
    errUnwrap, ok := (err).(interface {Unwrap() []error})
    if !ok {
        panic("not unwrap")
    }

    // Panics are errors
    errs := errUnwrap.Unwrap()
    for _, err := range errs {
        if rPanic, ok := sentinel.IsPanicError(err); ok {
            fmt.Printf("panic value: %v\n", rPanic)
        }
    }
}
```

Panics are always recorded with `panics_total` and `errors_total` counters.

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
    
    // Your application code...
    for range time.NewTicker(3 * time.Second).C {
        err := observer.RunFunc(doFunc)
        if err != nil {
            fmt.Printf("error occurred: %v\n", err)
        }
    }
}
```

Prometheus metrics will be exposed with names `myapp_workers_...` on host _localhost:8080/metrics_.

## Contributing

Please report any issues or feature requests to the [GitHub repository](https://github.com/mcwalrus/go-sentinel).

## About

This module is maintained by Max Collier under an MIT License Agreement.