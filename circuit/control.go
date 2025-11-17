package circuit

// ExecutionPhase represents the phase of task execution when a control check occurs.
type ExecutionPhase int

const (
	// PhaseNewRequest represents the initial execution attempt before any retries.
	// This phase occurs when a new task is about to be executed for the first time.
	PhaseNewRequest ExecutionPhase = iota
	// PhaseRetry represents a retry attempt after a previous failure.
	// This phase occurs when a task is being retried after encountering an error.
	PhaseRetry
)

// Control determines whether attempt to run task should stop either before the initial
// attempt or any retry attempt. It returns true to stop further attempts to run tasks.
// False will allow attempts to continue.
//
// The phase parameter indicates whether this is a new request (PhaseNewRequest) or
// a retry attempt (PhaseRetry), allowing different logic for each phase.
//
// Example usage:
//
//	control := func(phase circuit.ExecutionPhase) bool {
//		if phase == circuit.PhaseNewRequest {
//			return !shouldAcceptNewRequests()
//		}
//		return false
//	}
type Control func(phase ExecutionPhase) (shouldStop bool)

// OnDone stops when the provided done channel is closed.
// This can be used to represent a full-closed control signal, typically for graceful shutdown.
// Once the channel is closed, all new requests and retries will be stopped.
//
// Example usage:
//
//	done := make(chan struct{})
//	observer.UseConfig(sentinel.ObserverConfig{
//		Control: circuit.WhenDone(done),
//	})
//	// Later, when shutting down:
//	close(done) // This will stop all new requests and retries
func WhenDone(done <-chan struct{}) Control {
	return func(_ ExecutionPhase) bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
}
