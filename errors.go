package sentinel

import "errors"

// ErrPanicOccurred is the error returned when a panic occurs and is recovered.
var ErrPanicOccurred = errors.New("panic occurred for task execution")
