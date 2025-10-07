package sentinel

import "errors"

// ErrRecoveredPanic is the error returned when a panic occurs and is recovered
// by the [Observer]. The panic value can be retrieved from the error directly.
type ErrRecoveredPanic struct {
	panic any
}

// Error implements the error interface.
func (e *ErrRecoveredPanic) Error() string {
	return "panic recovered during task execution"
}

// RecoveredPanic returns the original panic value that was recovered.
func (e *ErrRecoveredPanic) RecoveredPanic() any {
	return e.panic
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
	var target = &ErrRecoveredPanic{}
	ok := errors.As(err, &target)
	return target.panic, ok
}
