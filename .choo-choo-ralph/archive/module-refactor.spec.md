---
title: "go-sentinel Module Refactor (v2)"
created: 2026-03-14
poured:
  - go-sentinel-mol-xhyl
  - go-sentinel-mol-y4tl
  - go-sentinel-mol-rk79
  - go-sentinel-mol-8lyk
  - go-sentinel-mol-252g
  - go-sentinel-mol-c8pm
  - go-sentinel-mol-krwj
  - go-sentinel-mol-g4p7
  - go-sentinel-mol-q1l1
  - go-sentinel-mol-ndtx
  - go-sentinel-mol-l3fy
  - go-sentinel-mol-gwwa
  - go-sentinel-mol-wmnl
  - go-sentinel-mol-4o9r
  - go-sentinel-mol-exlp
  - go-sentinel-mol-6cfb
  - go-sentinel-mol-h280
  - go-sentinel-mol-9qgd
  - go-sentinel-mol-3si5
  - go-sentinel-mol-56sd
  - go-sentinel-mol-fwerl
  - go-sentinel-mol-zocwh
  - go-sentinel-mol-hxatf
  - go-sentinel-mol-kszrw
  - go-sentinel-mol-tu4dn
  - go-sentinel-mol-05ljw
  - go-sentinel-mol-z2buk
  - go-sentinel-mol-cn8wu
  - go-sentinel-mol-rqmrm
  - go-sentinel-mol-kho86
  - go-sentinel-mol-lvwf6
  - go-sentinel-mol-dm25r
  - go-sentinel-mol-b8r15
  - go-sentinel-mol-z0ive
  - go-sentinel-mol-fyfrr
  - go-sentinel-mol-95rg3
iteration: 1
auto_discovery: false
auto_learnings: false
---
<project_specification>
<project_name>go-sentinel Module Refactor (v2)</project_name>

  <overview>
    Major breaking refactor of go-sentinel to introduce a composable, opt-in builder API for metrics,
    an async Submit() path powered by sourcegraph/conc, a clean Retrier interface, and comprehensive
    godoc/doc.go documentation. Observer becomes a proper subset of VecObserver. The circuit breaker
    is removed/demoted. This release bumps the major version to v2.
  </overview>

  <context>
    <existing_patterns>
      - NewObserver(durationBuckets, ...ObserverOption) pattern — options use WithXxx() naming
      - ObserverOption is a func(config) applied at construction time in config.go
      - Metrics are opt-in today via WithMetrics(MetricXxx...) string constants — the refactor replaces this with typed builder funcs
      - Execution flow: limiter → control check → in-flight → timeout ctx → execute → retry recursion → success/error recording
      - Panic recovery is already implemented; safeXxx wrappers exist for all pluggable funcs
      - VecObserver.With() / WithLabels() returns a new Observer sharing the underlying vec metrics
      - Tests use prometheus.NewRegistry() + testutil.ToFloat64() for metric assertions; helper Verify(t, obs, counts) pattern
      - t.Parallel() used throughout tests
    </existing_patterns>
    <integration_points>
      - observer.go — core Run/RunFunc/execute loop; primary site for Submit() addition
      - observer_config.go — ObserverConfig struct; construction-time changes land here
      - metrics.go — all metric structs; gets new typed WithXxxMetrics() funcs replacing string-based WithMetrics()
      - config.go — ObserverOption wiring; gains new builder options
      - vec_observer.go — shares interface with Observer; gains inline overlapping interface definitions
      - limiter.go — channel semaphore; stays but backed by sourcegraph/conc pool for Submit()
      - errors.go — ErrControlBreaker, RecoveredPanic; may need new error types for Submit() failures
      - circuit/ and retry/ subpackages — circuit is demoted; retry stays and gains Retrier interface
    </integration_points>
    <new_technologies>
      - github.com/sourcegraph/conc — structured concurrency library; use conc/pool for Submit() worker pools; handles panic propagation and goroutine lifecycle cleanly
    </new_technologies>
    <conventions>
      - Package: sentinel (github.com/mcwalrus/go-sentinel)
      - Constructors: NewTypeName(); options: WithFieldName()
      - Internal types lowercase, exported PascalCase
      - Metric constants: MetricXxx pattern (to be replaced by typed funcs but constants may stay for compat in transition)
      - Godoc: every exported identifier needs a comment; doc.go for package-level
      - Tests colocated at package root (*_test.go); benchmarks in benchmarks_test.go
    </conventions>
  </context>

  <tasks>

    <task id="add-conc-dep" priority="0" category="infrastructure">
      <title>Add sourcegraph/conc Dependency</title>
      <description>
        Add github.com/sourcegraph/conc to go.mod and go.sum so it can be used
        for structured concurrency in Submit() and the new worker pool.
      </description>
      <steps>
        - Run `go get github.com/sourcegraph/conc` to add the dependency
        - Verify go.mod and go.sum are updated
        - Run `go build ./...` to confirm no breakage
      </steps>
      <test_steps>
        1. Run `go build ./...` — expect no errors
        2. Run `go test ./...` — expect all existing tests pass
      </test_steps>
      <review></review>
    </task>

    <task id="composable-metrics-api" priority="1" category="functional">
      <title>Composable Opt-In Metrics Builder API</title>
      <description>
        Replace the string-based WithMetrics(MetricXxx...) option with typed builder functions
        that each enable a specific set of metrics. Each WithXxxMetrics() call is an ObserverOption.
        Keep internal metric structs but wire them through the new builder pattern.

        Builder functions to add:
        - WithInFlightMetrics() — enables in_flight gauge
        - WithSuccessMetrics() — enables success_total counter
        - WithErrorMetrics() — enables errors_total + failures_total counters
        - WithPanicMetrics() — enables panics_total counter
        - WithRetryMetrics() — enables retries_total counter
        - WithTimeoutMetrics() — enables timeouts_total counter
        - WithDurationMetrics(buckets []float64) — enables durations_seconds histogram
        - WithQueueMetrics() — enables pending_total gauge (queue/async)

        Also add WithErrorLabels(labeler func(err error) prometheus.Labels) option that
        enables per-error-type labeling on the errors counter.
      </description>
      <steps>
        - Define each WithXxxMetrics() function in config.go as an ObserverOption
        - Each option sets a corresponding flag/config on the internal config struct
        - Update metrics.go to only register/expose metrics that are enabled
        - WithErrorLabels() stores a labeler func; metrics.go uses CounterVec when set
        - Remove or deprecate the old string-based WithMetrics() API
        - Ensure metric registration skips unregistered metrics (no Prometheus conflicts)
      </steps>
      <test_steps>
        1. Construct observer with only WithSuccessMetrics() — verify only success_total appears in gathered metrics
        2. Construct observer with WithDurationMetrics([]float64{0.1, 0.5}) — verify histogram is registered
        3. Construct observer with all options — verify all 9 metric families are registered
        4. Construct observer with no options — verify no metrics registered but observer still runs
        5. Run a task and verify counters increment only for enabled metrics
        6. WithErrorLabels: run a failing task, verify errors_total has expected label values
      </test_steps>
      <review></review>
    </task>

    <task id="new-observer-default" priority="1" category="functional">
      <title>NewObserverDefault() Constructor</title>
      <description>
        Introduce NewObserverDefault() as a convenience constructor that opts into the
        most common metric set: in-flight, success, and error metrics. This provides
        a migration path for existing users while the raw NewObserver() becomes fully opt-in.
      </description>
      <steps>
        - Add NewObserverDefault() func in observer.go
        - It calls NewObserver() with WithInFlightMetrics(), WithSuccessMetrics(), WithErrorMetrics()
        - Add godoc explaining this is the recommended starting point for most users
      </steps>
      <test_steps>
        1. Call NewObserverDefault(), run a successful task — verify in_flight, success_total, errors_total are present
        2. Verify durations_seconds, panics_total, etc. are NOT registered
        3. Confirm it satisfies the same interface as NewObserver()
      </test_steps>
      <review></review>
    </task>

    <task id="retrier-interface" priority="1" category="functional">
      <title>Introduce Retrier Interface; Demote Circuit Breaker</title>
      <description>
        Add a clean Retrier interface to the retry package:

        ```go
        type Retrier interface {
            Do(ctx context.Context, fn func() error) error
        }
        ```

        The Observer's retry logic should accept a Retrier so users can supply custom
        retry mechanisms. Provide a default implementation using the existing WaitFunc-based
        strategy.

        Demote the circuit/ package: remove it from the main execution path or make it
        purely opt-in. The RetryBreaker concept can remain as an option but should not be
        required. Do not expose ErrControlBreaker as a first-class error if circuit is demoted.
        Keep WhenClosed (graceful shutdown control) as it is useful; rename or move if needed.
      </description>
      <steps>
        - Define Retrier interface in retry/retry.go
        - Implement DefaultRetrier struct that wraps existing WaitFunc + MaxRetries logic
        - Add WithRetrier(r retry.Retrier) ObserverOption to replace MaxRetries/RetryStrategy/RetryBreaker fields
        - Keep existing WaitFunc-based helpers (Immediate, Linear, Exponential, etc.) as building blocks for DefaultRetrier
        - Move or keep circuit.WhenClosed as a standalone control option (not tied to circuit package)
        - Remove or mark circuit.After and circuit.OnPanic as internal/deprecated if circuit is fully demoted
        - Update ObserverConfig to use Retrier field instead of MaxRetries/RetryStrategy/RetryBreaker triplet
      </steps>
      <test_steps>
        1. Construct observer with WithRetrier(retry.NewDefaultRetrier(retry.Exponential(100ms), 3))
        2. Run a task that fails twice then succeeds — verify 2 retries recorded
        3. Implement a custom Retrier and pass it via WithRetrier — verify it is called
        4. Verify that when no Retrier is set, tasks are not retried (existing behavior)
        5. Test WhenClosed control still works for graceful shutdown
      </test_steps>
      <review></review>
    </task>

    <task id="construction-time-config" priority="2" category="functional">
      <title>Move UseConfig to Construction-Time Options</title>
      <description>
        Replace the mutable UseConfig(ObserverConfig) method with construction-time options.
        ObserverConfig fields (Timeout, MaxConcurrency, etc.) become ObserverOption funcs.
        The observer should be immutable after construction.

        Options to add:
        - WithTimeout(d time.Duration)
        - WithMaxConcurrency(n int)
        - WithControl(c circuit.Control) — or equivalent after circuit demotion
        - WithRetrier(r retry.Retrier) — from previous task

        Keep UseConfig() temporarily as a deprecated shim if needed, but flag it in godoc.
      </description>
      <steps>
        - Add WithTimeout, WithMaxConcurrency, WithControl as ObserverOption funcs in config.go
        - Remove or deprecate UseConfig() method on Observer
        - Update observer.go to read all config from the immutable config struct set at construction
        - Remove sync.RWMutex from Observer if UseConfig is gone (config no longer mutated post-construction)
        - Update all examples and tests to use construction-time options
      </steps>
      <test_steps>
        1. Construct observer with WithTimeout(500ms) — run a task that takes 600ms — verify timeout fires
        2. Construct observer with WithMaxConcurrency(2) — run 5 concurrent tasks — verify only 2 run at once
        3. Confirm Observer struct no longer needs RWMutex after this change
        4. Confirm all existing tests pass with updated construction pattern
      </test_steps>
      <review></review>
    </task>

    <task id="async-submit" priority="1" category="functional">
      <title>Async Submit() Using sourcegraph/conc</title>
      <description>
        Add Submit() method to Observer that executes a task asynchronously using a
        sourcegraph/conc worker pool. Submit() is fire-and-forget with metrics still
        recorded. All panics are captured and propagated safely via conc's panic handling.

        API sketch:
        ```go
        // Submit enqueues fn for async execution. Panics are captured and propagated
        // through the pool's Wait() mechanism.
        func (o *Observer) Submit(fn func() error)
        func (o *Observer) SubmitFunc(fn func(ctx context.Context) error)

        // Wait blocks until all submitted tasks complete and returns any collected errors.
        func (o *Observer) Wait() error
        ```

        Also update WithQueueMetrics() to track the async queue depth (pending_total).
      </description>
      <steps>
        - Add conc/pool.Pool field to Observer (initialized lazily or at construction with WithMaxConcurrency)
        - Implement Submit(fn func() error) and SubmitFunc(fn func(ctx context.Context) error)
        - Implement Wait() error that drains the pool
        - Ensure all metrics (in_flight, success, errors, durations) are still recorded for async tasks
        - WithQueueMetrics() should increment pending_total when Submit() enqueues and decrement when task starts
        - Panic recovery across Submit() uses conc's panic capture so Wait() surfaces them
        - Run() and RunFunc() remain fully synchronous (no change to signature/behavior)
      </steps>
      <test_steps>
        1. Submit() 10 tasks — call Wait() — verify all 10 success_total increments recorded
        2. Submit() a task that panics — Wait() should return an error wrapping the panic
        3. WithQueueMetrics() + Submit() — verify pending_total increments/decrements correctly
        4. Submit() with WithMaxConcurrency(2) — verify at most 2 run concurrently
        5. Submit() and Run() interleave — verify no metric or concurrency conflicts
      </test_steps>
      <review></review>
    </task>

    <task id="observer-vecobserver-interface" priority="2" category="functional">
      <title>Observer as Subset of VecObserver — Shared Interface</title>
      <description>
        Define explicit shared interfaces so Observer is clearly a subset of VecObserver.
        Move overlapping method signatures into an inline interface definition so both types
        satisfy it without duplication.

        Proposed interface:
        ```go
        // Executor is the core execution interface satisfied by both Observer and any labeled
        // Observer derived from a VecObserver.
        type Executor interface {
            Run(fn func() error) error
            RunFunc(fn func(ctx context.Context) error) error
            Submit(fn func() error)
            SubmitFunc(fn func(ctx context.Context) error)
            Wait() error
        }
        ```

        Observer should embed or satisfy Executor. VecObserver.With() / WithLabels() return
        a value that also satisfies Executor.
      </description>
      <steps>
        - Define Executor interface (or equivalent name) in a suitable file (observer.go or interfaces.go)
        - Ensure Observer satisfies it
        - Ensure the Observer returned by VecObserver.With() / WithLabels() satisfies it
        - Add compile-time interface checks: var _ Executor = (*Observer)(nil)
        - Update godoc to explain the relationship
      </steps>
      <test_steps>
        1. Compile-time: var _ Executor = (*Observer)(nil) — confirm no error
        2. Compile-time: var _ Executor = labeled observer from VecObserver.WithLabels() — confirm no error
        3. Use an Executor variable to call Run, Submit, Wait — verify dispatch works
      </test_steps>
      <review></review>
    </task>

    <task id="panic-handling" priority="1" category="functional">
      <title>Comprehensive Panic Handling Across All Boundaries</title>
      <description>
        Ensure panics are safely recovered at every execution boundary:
        - Synchronous Run() / RunFunc()
        - Async Submit() / SubmitFunc() (via conc pool)
        - All pluggable callbacks: Retrier.Do(), Control(), WaitFunc, ErrorLabeler
        - VecObserver.With() / WithLabels() construction

        Panics should be captured as RecoveredPanic errors and recorded via panics_total
        (when WithPanicMetrics() is enabled). DisablePanicRecovery(true) should still
        allow panics to propagate after recording the metric.
      </description>
      <steps>
        - Audit every goroutine and callback invocation for missing panic recovery
        - Add recover() wrappers to all pluggable func invocations (Retrier, Control, WaitFunc, ErrorLabeler)
        - For Submit(), conc pool handles goroutine panics — ensure RecoveredPanic wrapping is consistent
        - DisablePanicRecovery: if true, re-panic after recording metric (existing behavior — verify it still works)
        - Add tests that inject panics into each boundary
      </steps>
      <test_steps>
        1. Retrier.Do() panics — verify RecoveredPanic is returned as error, panics_total increments
        2. Control() panics — verify it is safely recovered and treated as closed/open (non-fatal)
        3. Submit() task panics — Wait() returns wrapped RecoveredPanic
        4. DisablePanicRecovery(true) + panicking task — verify panic propagates after metric increment
        5. Run all existing panic tests — verify no regressions
      </test_steps>
      <review></review>
    </task>

    <task id="godoc-and-doc-go" priority="2" category="documentation">
      <title>Godoc Comments and doc.go Package Overview</title>
      <description>
        Add godoc comments to every exported type, function, method, and constant.
        Create doc.go with a package-level overview and decision rationale so that
        users and AI agents can understand the library's purpose and design quickly.

        doc.go should cover:
        - What go-sentinel is (observability wrapper for Go functions)
        - Core concepts: Observer, VecObserver, metrics builder, Executor interface
        - Design decisions: why opt-in metrics, why Retrier interface, why conc for async
        - Quick-start code example
        - Link to examples/ directory
      </description>
      <steps>
        - Create doc.go in package root with package sentinel and full package doc comment
        - Add godoc to: Observer, VecObserver, Executor interface, all WithXxxMetrics() funcs,
          NewObserver, NewObserverDefault, Run, RunFunc, Submit, SubmitFunc, Wait,
          Retrier interface, DefaultRetrier, all retry strategy funcs, all exported constants
        - Ensure doc comments follow Go conventions (start with the identifier name)
        - Add a package-level example in examples/ or doc.go example func
      </steps>
      <test_steps>
        1. Run `go doc ./...` — verify no exported identifiers are missing docs
        2. Run `go vet ./...` — verify no doc comment format issues
        3. Read doc.go — verify it clearly explains the library to a newcomer in under 2 minutes
      </test_steps>
      <review></review>
    </task>

    <task id="update-examples-and-tests" priority="3" category="functional">
      <title>Update Examples and Tests for New API</title>
      <description>
        Update all existing examples and tests to use the new v2 API:
        - Replace UseConfig() with construction-time options
        - Replace WithMetrics(MetricXxx) with WithXxxMetrics() funcs
        - Update retry usage to use Retrier interface
        - Add Submit() examples to examples/ directory
        - Update workerloop example to demonstrate async Submit() + Wait()
      </description>
      <steps>
        - Update examples/multiple-observers/main.go — use new builder API
        - Update examples/vec-observer/main.go — use new builder API
        - Update examples/workerloop/main.go — demonstrate Submit() + Wait() pattern
        - Update all *_test.go files to use construction-time options
        - Add new test cases covering Submit(), Retrier, WithErrorLabels()
        - Remove tests that relied on UseConfig() mutation if removed
      </steps>
      <test_steps>
        1. Run `go test ./...` — all tests pass
        2. Run `go build ./examples/...` — all examples compile
        3. Run each example manually — verify output is sensible
        4. Run benchmarks: `go test -bench=. ./...` — verify no significant perf regression
      </test_steps>
      <review></review>
    </task>

    <task id="version-bump" priority="3" category="infrastructure">
      <title>Bump to v2 Major Version</title>
      <description>
        Update the module path to v2 to reflect the breaking API changes per Go module
        versioning conventions. Update go.mod, all internal imports, and documentation.
      </description>
      <steps>
        - Update go.mod module path to github.com/mcwalrus/go-sentinel/v2
        - Update all import paths in package source files
        - Update README and doc.go with v2 import path
        - Tag the release as v2.0.0 (or prepare for tagging)
      </steps>
      <test_steps>
        1. Run `go build ./...` with updated module path — no errors
        2. Run `go test ./...` — all tests pass
        3. Verify `go list -m` shows correct v2 module path
      </test_steps>
      <review></review>
    </task>

  </tasks>

  <success_criteria>
    - All exported types/methods have godoc; doc.go explains the library clearly
    - NewObserver() uses opt-in WithXxxMetrics() builder funcs; NewObserverDefault() exists
    - Run() / RunFunc() remain synchronous and backward-compatible in behavior
    - Submit() / SubmitFunc() / Wait() provide async execution via sourcegraph/conc
    - Retrier interface accepted as a first-class option; DefaultRetrier wraps existing strategies
    - Circuit breaker removed from main path; WhenClosed control preserved
    - Observer is a proper subset of VecObserver; both satisfy Executor interface
    - Panics safely captured at every boundary
    - Module path updated to v2
    - All tests pass; no significant benchmark regression
  </success_criteria>

</project_specification>
