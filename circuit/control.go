package circuit

// Control determines whether attempt to run task should stop either before the initial
// attempt or any retry attempt. It returns true to stop further attempts to run tasks.
// False will allow attempts to continue.
type Control func() bool

// OnDone stops when the provided done channel is closed.
// This can be used to represent a full-closed control signal.
func OnDone(done <-chan struct{}) Control {
	return func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}
}
