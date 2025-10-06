package sentinel

// ErrPanicOccurred is the error returned when a panic occurs and is recovered
// by the [Observer]. The panic value can be retrieved from the error directly.
type ErrPanicOccurred struct {
	panic any
}

// Error implements the error interface.
func (e *ErrPanicOccurred) Error() string {
	return "panic occurred for task execution"
}

// PanicValue returns the panic value.
func (e *ErrPanicOccurred) PanicValue() any {
	return e.panic
}
