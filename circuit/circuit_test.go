package circuit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubPanicErr simulates sentinel.ErrRecoveredPanic (error + RecoveredPanic())
type stubPanicErr struct{ v any }

func (e *stubPanicErr) Error() string       { return "panic occurred" }
func (e *stubPanicErr) RecoveredPanic() any { return e.v }

func TestOnPanic(t *testing.T) {
	t.Parallel()

	b := OnPanic()
	if b(nil) {
		t.Fatalf("nil error should not trip")
	}
	if b(errors.New("x")) {
		t.Fatalf("regular error should not trip")
	}
	if !b(&stubPanicErr{v: "boom"}) {
		t.Fatalf("panic-like error should trip")
	}
}

func TestAfter(t *testing.T) {
	t.Parallel()

	b := After(5 * time.Millisecond)
	if b(errors.New("x")) {
		t.Fatalf("should not trip immediately")
	}
	time.Sleep(10 * time.Millisecond)
	if !b(errors.New("x")) {
		t.Fatalf("should trip after duration")
	}
}

func TestAny(t *testing.T) {
	t.Parallel()

	b := Any(
		OnErrorIs(context.Canceled),
		OnErrors(context.DeadlineExceeded),
	)
	if b(nil) {
		t.Fatalf("nil should not trip")
	}
	if b(errors.New("nope")) {
		t.Fatalf("unrelated error should not trip")
	}
	if !b(context.Canceled) {
		t.Fatalf("canceled should trip")
	}
	if !b(context.DeadlineExceeded) {
		t.Fatalf("deadline exceeded should trip")
	}
}

func TestOnError(t *testing.T) {
	t.Parallel()

	b := OnError(func(err error) bool { return errors.Is(err, context.DeadlineExceeded) })
	if b(nil) {
		t.Fatalf("nil should not trip")
	}
	if b(errors.New("x")) {
		t.Fatalf("unrelated should not trip")
	}
	if !b(context.DeadlineExceeded) {
		t.Fatalf("deadline exceeded should trip")
	}
}

func TestOnErrorIs(t *testing.T) {
	t.Parallel()

	b := OnErrorIs(context.Canceled)
	if !b(context.Canceled) {
		t.Fatalf("should trip on target")
	}
	if b(context.DeadlineExceeded) {
		t.Fatalf("should not trip on other")
	}
}

type customErr struct{ msg string }

func (e *customErr) Error() string { return e.msg }

func TestOnErrorAs(t *testing.T) {
	t.Parallel()

	b := OnErrorAs[*customErr]()
	if b(errors.New("x")) {
		t.Fatalf("should not trip on non-target type")
	}
	if !b(&customErr{"x"}) {
		t.Fatalf("should trip on target type")
	}
}

func TestOnSignalAndDone(t *testing.T) {
	t.Parallel()

	sig := make(chan struct{}, 1)
	bSig := OnSignal(sig)
	bDone := OnDone(sig)
	if bSig(errors.New("x")) || bDone(errors.New("x")) {
		t.Fatalf("should not trip before signal")
	}
	sig <- struct{}{}
	if !bSig(errors.New("x")) || !bDone(errors.New("x")) {
		t.Fatalf("should trip after signal")
	}
}
