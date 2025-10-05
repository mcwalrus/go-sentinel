package sentinel

// ErrPanicOccurred is the error returned when a panic occurs and is recovered.
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
