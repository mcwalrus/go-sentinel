package circuit

import (
	"testing"
)

func TestOnSignalAndDone(t *testing.T) {
	t.Parallel()

	sig := make(chan struct{}, 1)
	bDone := OnDone(sig)
	if bDone(PhaseNewRequest) {
		t.Fatalf("should not trip before signal")
	}

	sig <- struct{}{}
	if !bDone(PhaseNewRequest) {
		t.Fatalf("should trip after signal read")
	}
	if bDone(PhaseNewRequest) {
		t.Fatalf("should not trip after signal read")
	}

	close(sig)
	if !bDone(PhaseNewRequest) {
		t.Fatalf("should trip after chan closed")
	}
	if !bDone(PhaseNewRequest) {
		t.Fatalf("should not trip after chan closed")
	}
}
