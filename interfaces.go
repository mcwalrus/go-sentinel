package sentinel

import "context"

// Executor is the core execution interface satisfied by Observer and any labeled
// observer derived from a VecObserver.
type Executor interface {
	Run(fn func() error) error
	RunFunc(fn func(ctx context.Context) error) error
	Submit(fn func() error)
	SubmitFunc(fn func(ctx context.Context) error)
	Wait() error
}

// Compile-time assertion that *Observer satisfies Executor.
// Observers returned by VecObserver.With and VecObserver.WithLabels are also
// *Observer and therefore satisfy this interface.
var _ Executor = (*Observer)(nil)
