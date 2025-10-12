package circuit

import (
	"testing"
)

func TestOnSignalAndDone(t *testing.T) {
	t.Parallel()

	sig := make(chan struct{}, 1)
	bDone := OnDone(sig)
	if bDone() {
		t.Fatalf("should not trip before signal")
	}

	sig <- struct{}{}
	if !bDone() {
		t.Fatalf("should trip after signal read")
	}
	if bDone() {
		t.Fatalf("should not trip after signal read")
	}

	close(sig)
	if !bDone() {
		t.Fatalf("should trip after chan closed")
	}
	if !bDone() {
		t.Fatalf("should not trip after chan closed")
	}
}
