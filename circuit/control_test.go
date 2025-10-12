package circuit

import (
	"testing"
)

func TestOnSignalAndDone(t *testing.T) {
	t.Parallel()

	sig1 := make(chan struct{}, 1)
	bSig := OnSignal(sig1)
	sig2 := make(chan struct{}, 1)
	bDone := OnDone(sig2)

	if bSig() || bDone() {
		t.Fatalf("should not trip before signal")
	}

	sig1 <- struct{}{}
	close(sig2)
	if !bSig() || !bDone() {
		t.Fatalf("should trip after signal")
	}
}

func TestAnyControl(t *testing.T) {
	t.Parallel()

	t.Run("nil control", func(t *testing.T) {
		b := AnyControl(nil, func() bool { return false })
		if b() {
			t.Fatalf("should not trip on nil control")
		}
	})

	t.Run("any control", func(t *testing.T) {
		sig1 := make(chan struct{}, 1)
		bSig := OnSignal(sig1)
		sig2 := make(chan struct{}, 1)
		bDone := OnDone(sig2)

		if AnyControl(bSig, bDone)() {
			t.Fatalf("should not trip before signal")
		}
		if !AnyControl(bSig, bDone, func() bool { return true })() {
			t.Fatalf("should trip on constant control")
		}
		close(sig1)
		close(sig2)
		if !AnyControl(bSig, bDone, func() bool { return false })() {
			t.Fatalf("should trip on any control")
		}
	})
}
