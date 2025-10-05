package sentinel

// ErrPanicOccurred is the error returned when a panic occurs and is recovered.
type ErrPanicOccurred struct {
	panic interface{}
}

// Error implements the error interface.
func (e ErrPanicOccurred) Error() string {
	return "panic occurred for task execution"
}

// Panic returns the panic value.
func (e ErrPanicOccurred) Panic() interface{} {
	return e.panic
}
