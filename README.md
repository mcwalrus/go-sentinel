# Sentinel

[![Go Version](https://img.shields.io/github/go-mod/go-version/mcwalrus/go-sentinel)](https://golang.org/)
[![Go Report Card](https://goreportcard.com/badge/github.com/mcwalrus/go-sentinel)](https://goreportcard.com/report/github.com/mcwalrus/go-sentinel)
[![codecov](https://codecov.io/gh/mcwalrus/go-sentinel/branch/main/graph/badge.svg)](https://codecov.io/gh/mcwalrus/go-sentinel) 
[![GoDoc](https://godoc.org/github.com/mcwalrus/go-sentinel?status.svg)](https://godoc.org/github.com/mcwalrus/go-sentinel)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

Sentinel wraps Go functions to automatically expose Prometheus metrics with reliability handlers. For functions, the library will: recover panic occurances as errors, configure retry handling, and track metrics of successes, errors, panics, retries, timeouts, and execution durations. Sentinel is designed to be minimal, robust, and immediately integrate with existing applications.

## Metrics

Default configurations will automatically export the following Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `sentinel_in_flight` | Gauge | Active number of running tasks |
| `sentinel_successes_total` | Counter | Total successful tasks |
| `sentinel_failures_total` | Counter | Total failed tasks |
| `sentinel_errors_total` | Counter | Total errors over all attempts |
| `sentinel_panics_total` | Counter | Total panic occurrences |
| `sentinel_durations_seconds` | Histogram | Task execution durations in buckets |
| `sentinel_timeouts_total` | Counter | Total errors based on timeouts |
| `sentinel_retries_total` | Counter | Total retry attempts for tasks |

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
    // Handle error
    if err != nil {
        log.Printf("Task failed: %v\n", err)
    }
}
```

### Failure Handlers

Observer records errors via metrics with returning errors:

```go
package main

import (
    "errors"
    "log"
    
    sentinel "github.com/mcwalrus/go-sentinel"
)

func main() {
    // New observer
    observer := sentinel.NewObserver(nil)
    
    // Task fails
    err := observer.Run(func() error {
        return errors.New("task failed")
    })
    // Handle error
    if err != nil {
        log.Printf("Task failed: %v\n", err)
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

    // Task respects context timeout
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
    "math/rand"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
)

func main() {
    // New observer
    observer := sentinel.NewObserver(nil)
    
    // Task panics
    err := observer.Run(func() error {
        panic("stations!:0")
    })
    
    // Handle error
    if err != nil {
        log.Printf("Task failed: %v\n", err)
    }

    // Recover panic value
    if r, ok := sentinel.IsPanicError(err); ok {
        log.Printf("panic value: %v\n", r)
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
    "log"
    
    sentinel "github.com/mcwalrus/go-sentinel"
)

func main() {
    // New observer
    observer := sentinel.NewObserver(nil)
    
    // Disabled recovery
    observer.DisablePanicRecovery(true)
    
    // Panic propogates
    err := observer.Run(func() error {
        panic("some failure")
    })

    // Unreachable code
    log.Printf("err was: %v\n", err)
}
```

### Observe Durations

Set histogram buckets with the observer to export `durations_seconds` metrics:

```go
package main

import (
    "context"
    "errors"
    "log"
    "math/rand"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
)

func main() {
    // New observer with durations
    observer := sentinel.NewObserver(
        []float64{0.100, 0.250, 0.400, 0.500, 1.000}, // in seconds
    )
    
    // Run tasks for 50-1000ms before returning
    for i := 0; i < 100; i++ {
        _ = observer.RunFunc(func(ctx context.Context) error {
            sleep := time.Duration(rand.Intn(950)+50) * time.Millisecond
            log.Printf("Sleeping for %v...\n", sleep)
            time.Sleep(sleep)
            return nil
        })
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
    "log"
    "math/rand"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/mcwalrus/go-sentinel/retry"
)

func main() {
    // New observer
    observer := sentinel.NewObserver(nil)

    // Set retry behavior
    observer.UseConfig(sentinel.ObserverConfig{
        MaxRetries:    3,
        RetryStrategy: retry.WithJitter(
            time.Second,
            retry.Exponential(100*time.Millisecond),
        ),
    })

    // Fail every attempt
    err := observer.Run(func() error {
        return errors.New("task failed")
    })
    
    // Unwrap join errors
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

Use `sentinel.RetryCount(ctx)` to read the current retry attempt count within an observed function.

### Circuit Breaker Support

Configure circuit breaker to stop retries based on particular errors:

```go
package main

import (
    "errors"
    "log"
    "time"
    
    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/mcwalrus/go-sentinel/circuit"
    "github.com/mcwalrus/go-sentinel/retry"
)

var ErrCustom = errors.New("unrecoverable error")

func main() {
    observer := sentinel.NewObserver(nil)

    // Configure circuit breaker
    observer.UseConfig(sentinel.ObserverConfig{
        MaxRetries: 5,
        RetryBreaker: func(err error) bool {
            return errors.Is(err, ErrCustom)
        },
    })
    
    // Task returns custom error
    var count int
    err := observer.Run(func() error {
        count++
        return ErrCustom
    })
    if err != nil && count == 1 {
        log.Printf("Task stopped early: %v\n", err)
    }
}
```

### Prometheus Integration

Use template for integrating sentinel with a prometheus endpoint:

```go
import (
    "log"
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
            log.Fatal(err)
        }
    }()

    // Your application code
    for range time.NewTicker(3 * time.Second).C {
        err := observer.Run(doFunc)
        if err != nil {
            log.Printf("error occurred: %v\n", err)
        }
    }
}
```

Prometheus metrics will be exposed with names `myapp_workers_...` on host _localhost:8080/metrics_.

## Advanced Usage

### Labeled Observers

VecObserver enables creating multiple observers that share the same underlying metrics but are differentiated by Prometheus labels:

```go
package main

import (
    "net/http"
    "time"

    sentinel "github.com/mcwalrus/go-sentinel"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    // Create VecObserver with label names
    vecObserver := sentinel.NewVecObserver(
        []float64{0.1, 0.5, 1, 2, 5},
        []string{"service", "pipeline"},
    )
    
    // Register VecObserver metrics just once
    registry := prometheus.NewRegistry()
    vecObserver.MustRegister(registry)

    // Create observers with different labels
    mObserver, _ := vecObserver.WithLabels("api", "main")
    bgObserver, _ := vecObserver.WithLabels("api", "background")

    // Set observer configurations
    mObserver.UseConfig(sentinel.ObserverConfig{
        Timeout:    60 * time.Second,
        MaxRetries: 2,
    })
    bgObserver.UseConfig(sentinel.ObserverConfig{
        Timeout:    120 * time.Second,
        MaxRetries: 4,
    })

    // Use observers
    _ = prodObserver.Run(func() error {
        return nil
    })    
    _ = stagingObserver.Run(func() error {
        return nil
    })
}
```

Using `VecObserver` instead of creating multiple `Observer`s would be recommended best practise for most cases.

## Contributing

Please report any issues or feature requests to the [GitHub repository](https://github.com/mcwalrus/go-sentinel).

I am particularly keen to hear feedback around how to appropriately present the library alongside issues.

Please reach out to me directly for issues which require urgent fixes.

## About

This module is maintained by Max Collier under an MIT License Agreement.