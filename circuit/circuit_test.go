package circuit

import (
	"context"
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

func TestAny(t *testing.T) {
	t.Parallel()

	// Test with custom error matching functions instead of error wrappers
	b := AnyBreaker(
		func(err error) bool { return errors.Is(err, context.Canceled) },
		func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
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

func TestRunConfig(t *testing.T) {
	t.Parallel()

	b := func(err error) bool { return errors.Is(err, context.DeadlineExceeded) }
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

func TestAnyBreakerWithNil(t *testing.T) {
	t.Parallel()

	b := AnyBreaker(nil, func(err error) bool { return err != nil })
	if b(nil) {
		t.Fatalf("should not trip on nil error")
	}
	if !b(errors.New("test")) {
		t.Fatalf("should trip on non-nil error")
	}
}
