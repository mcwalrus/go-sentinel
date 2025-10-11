package circuit

// Control determines whether attempt to run task should stop either before the initial
// attempt or any retry attempt. It returns true to stop further attempts to run tasks.
// False will allow attempts to continue.
type Control func() bool

// OnSignal stops when a value is received on the provided channel.
// The channel is read in a non-blocking way to keep the predicate fast.
func OnSignal[T any](ch <-chan T) Control {
	return func() bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
}

// OnDone stops when the provided done channel is closed.
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

// AnyControl returns a control that stops when
// any of the provided controls return true.
func AnyControl(cs ...Control) Control {
	return func() bool {
		for _, c := range cs {
			if c != nil && c() {
				return true
			}
		}
		return false
	}
}
