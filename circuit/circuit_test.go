package circuit

import (
	"errors"
	"testing"
	"time"
)

type stubPanicErr struct{ v any }

func (e *stubPanicErr) Error() string { return "panic occurred" }
func (e *stubPanicErr) Value() any    { return e.v }

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
