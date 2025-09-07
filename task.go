package viewprom

import "context"

type Task interface {
	Config() Config
	Execute(ctx context.Context) error
}

type implJob struct {
	fn  func(ctx context.Context) error
	cfg Config
}

func (j *implJob) Config() Config {
	return j.cfg
}

func (j *implJob) Execute(ctx context.Context) error {
	return j.fn(ctx)
}
