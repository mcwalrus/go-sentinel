package viewprom

import "context"

type Task interface {
	Config() Config
	Execute(ctx context.Context) error
}

type implTask struct {
	fn  func(ctx context.Context) error
	cfg Config
}

func (j *implTask) Config() Config {
	return j.cfg
}

func (j *implTask) Execute(ctx context.Context) error {
	return j.fn(ctx)
}
