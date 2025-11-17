package sentinel

import (
	"errors"
	"runtime"
	"runtime/debug"
)

// ErrControlBreaker is the error returned when a control breaker is triggered.
// This occurs when ObserverConfig.Control returns true, preventing task execution
// before it even begins (during PhaseNewRequest) or stopping retry attempts
// (during PhaseRetry).
type ErrControlBreaker struct{}

func (e *ErrControlBreaker) Error() string {
	return "observer: control breaker avoided task execution entirely"
}

// ErrRecoveredPanic is the error returned when a panic occurs and is recovered
// by the [Observer]. The panic value can be retrieved from the error directly.
type RecoveredPanic struct {
	panic   any
	callers []uintptr
	stack   []byte
}

func newRecoveredPanic(skip int, value any) RecoveredPanic {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(skip+2, pcs)
	for n == len(pcs) {
		pcs = make([]uintptr, len(pcs)*2)
		n = runtime.Callers(skip+2, pcs)
	}
	pcs = pcs[:n]

	return RecoveredPanic{
		panic:   value,
		callers: pcs,
		stack:   debug.Stack(),
	}
}

func (e RecoveredPanic) Error() string {
	return "observer: panic recovery made during function execution"
}

// Value returns the original panic value that was recovered.
// This is the value that was passed to panic() when the panic occurred.
func (e RecoveredPanic) Value() any {
	return e.panic
}

// Callers returns the program counters of the call stack at the time of panic.
// This can be used with runtime.CallersFrames to inspect the call stack.
func (e RecoveredPanic) Callers() []uintptr {
	return e.callers
}

// Stack returns the stack trace captured at the time of panic recovery.
// The stack trace is formatted as a byte slice, similar to debug.Stack().
func (e RecoveredPanic) Stack() []byte {
	return e.stack
}

// IsPanicError checks if the given error was caused by a panic that was recovered
// by the [Observer]. It returns the panic value and a boolean indicating whether
// a panic occurred.
//
// Example usage:
//
//	err := observer.RunTask(myTask)
//	if panicValue, isPanic := IsPanicError(err); isPanic {
//		log.Printf("Task panicked with value: %v", panicValue)
//		// Handle panic-specific logic
//	} else if err != nil {
//		log.Printf("Task failed with error: %v", err)
//		// Handle regular error
//	}
func IsPanicError(err error) (any, bool) {
	var target = RecoveredPanic{}
	ok := errors.As(err, &target)
	return target.panic, ok
}
