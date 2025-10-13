# Sentinel

[![Go Version](https://img.shields.io/github/go-mod/go-version/mcwalrus/go-sentinel)](https://golang.org/)
[![Go Report Card](https://goreportcard.com/badge/github.com/mcwalrus/go-sentinel)](https://goreportcard.com/report/github.com/mcwalrus/go-sentinel)
[![codecov](https://codecov.io/gh/mcwalrus/go-sentinel/branch/main/graph/badge.svg)](https://codecov.io/gh/mcwalrus/go-sentinel) 
[![GoDoc](https://godoc.org/github.com/mcwalrus/go-sentinel?status.svg)](https://godoc.org/github.com/mcwalrus/go-sentinel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Sentinel provides retry handling with observerable metrics for Go applications. **Consider how panics should be handled as critical processes in production systems**. Sentinel wraps function execution to recover panics as errors while tracking Prometheus metrics for successes, errors, panics, retries, timeouts and durations - making routines to become more resilient, observable and robust, right out of the box. Use the library as a simple drop-in solution for new projects or existing applications.

## Features

- **Prometheus Metrics**: Observe tasks through pre-defined metrics
- **Composable Pattern**: Multiple observers can be employed at once
- **Retry Logic**: Enables retry strategies with circuit breaking support
- **Panic Recovery**: Safe panic recovery or standard panic propagation
- **Context Timeout**: Timeout support for handling task deadlines

## Metrics

Standard configuration will automatically export the following observer metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `sentinel_in_flight` | Gauge | Active number of running tasks |
| `sentinel_successes_total` | Counter | Total successful tasks |
| `sentinel_failures_total` | Counter | Total failed tasks |
| `sentinel_errors_total` | Counter | Total errors over all attempts |
| `sentinel_panics_total` | Counter | Total panic occurrences |
| `sentinel_timeouts_total` | Counter | Total errors based on timeouts |
| `sentinel_retries_total` | Counter | Total retry attempts for tasks |
| `sentinel_durations_seconds` | Histogram | Task execution durations in buckets |

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
    // New observer
    observer := sentinel.NewObserver(nil)
    
    // Execute task
    err := observer.Run(func() error {
        log.Println("Processing task...")
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
    // New observer
    observer := sentinel.NewObserver(nil)
    
    // Tracks task error
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
    // New observer
    observer := sentinel.NewObserver(nil)

    // Set tasks timeout
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
    // New observer
    observer := sentinel.NewObserver(nil)
    
    // Panic in observer
    err := observer.Run(func() error {
        panic("stations")
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

### Disable Panic Recovery

By default, the observer recovers panics and converts them to errors. You can disable this behavior to let panics propagate normally:

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    sentinel "github.com/mcwalrus/go-sentinel"
)

func main() {
    // New observer
    observer := sentinel.NewObserver(nil)
    
    // Disable panic recovery
    observer.DisableRecovery(true)
    
    // Panic will crash the program
    err := observer.Run(func() error {
        panic("some failure")
    })
    // Unreachable code ...
    if err != nil {
        log.Printf("Task failed: %v", err)
    }
}
```

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
    // New observer with durations
    observer := sentinel.NewObserver(
        []float64{0.100, 0.250, 0.400, 0.500, 1.000}, // in seconds
    )
    
    // Run tasks 50-500ms before returning
    for i := 0; i < 100; i++ {
        err := observer.RunFunc(func(ctx context.Context) error {
            sleep := time.Duration(rand.Intn(450)+50) * time.Millisecond
            fmt.Printf("Sleeping for %v...\n", sleep)
            time.Sleep(sleep)
            return nil
        })
        // Handle task errors
        if err != nil {
            log.Printf("Task failed: %d: %v", i, err)
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
    // New observer
    observer := sentinel.NewObserver(nil)

    // Configure retry strategy
    observer.UseConfig(sentinel.ObserverConfig{
        MaxRetries:    3,
        RetryStrategy: retry.WithJitter(
            retry.Exponential(100*time.Millisecond),
            time.Second,
        ),
    })

    // Fail on every attempt
    err := observer.Run(func() error {
        return errors.New("task failed")
    })
    
    // Unwrap errors
    errUnwrap, ok := (err).(interface {Unwrap() []error})
    if !ok {
        panic("not unwrap")
    }

    // Handle each error
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
    // New observer
    observer := sentinel.NewObserver(nil,
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

## Advanced Usage

### Forked Observers

You can fork observers to provide different behaviour with shared underlying metrics:

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
    // New observer
    observer := sentinel.NewObserver(nil,
        sentinel.WithNamespace("myapp"),
        sentinel.WithSubsystem("processes"),
    )

    // Only register observer
    registry := prometheus.NewRegistry()
    observer.MustRegister(registry)

    // Create forked observer
    forkedObserver := observer.Fork()

    // Set configurations per observer
    observer.UseConfig(sentinel.ObserverConfig{
        Timeout:    60 * time.Second,
        MaxRetries: 2,
    })
    forkedObserver.UseConfig(sentinel.ObserverConfig{
        Timeout:    120 * time.Second,
        MaxRetries: 4,
    })

    // Use observers ...
}
```

### Multiple Observers

You can use multiple observers for different types of tasks with separate metrics:

```go
package main

import (
    "context"
    "fmt"
    "log"
    "math/rand"
    "net/http"
    "time"

    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/mcwalrus/go-sentinel/retry"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // First observer
    bgObserver := sentinel.NewObserver(
        []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
        sentinel.WithNamespace("myapp"),
        sentinel.WithSubsystem("background"),
        sentinel.WithDescription("Background processing tasks"),
    )
    // Second observer
    criticalObserver := sentinel.NewObserver(
        []float64{0.01, 0.1, 0.5, 1, 5, 10, 30},
        sentinel.WithNamespace("myapp"),
        sentinel.WithSubsystem("critical_jobs"),
        sentinel.WithDescription("Critical business operations"),
    )
    // Register both observers
    registry := prometheus.NewRegistry()
    bgObserver.MustRegister(registry)
    criticalObserver.MustRegister(registry)

    // Set configurations per observer
    bgObserver.UseConfig(sentinel.ObserverConfig{
        Timeout:       30 * time.Second,
        MaxRetries:    3,
    })
    criticalObserver.UseConfig(sentinel.ObserverConfig{
        Timeout:    5 * time.Second,
        MaxRetries: 2,
        RetryStrategy: retry.Exponential(500 * time.Millisecond),
    })

    // Use observers ...
}
```

## Contributing

Please report any issues or feature requests to the [GitHub repository](https://github.com/mcwalrus/go-sentinel).

I am particularly keen to hear feedback around how to appropriately present the library alongside issues.

## About

This module is maintained by Max Collier under an MIT License Agreement.