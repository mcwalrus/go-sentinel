package viewprom

import (
	"context"
	"time"
)

type Task interface {
	Config() TaskConfig
	Execute(ctx context.Context) error
}

type implTask struct {
	fn  func(ctx context.Context) error
	cfg TaskConfig
}

func (j *implTask) Config() TaskConfig {
	return j.cfg
}

func (j *implTask) Execute(ctx context.Context) error {
	return j.fn(ctx)
}

type TaskConfig struct {
	Timeout       time.Duration
	MaxRetries    int
	RecoverPanics bool
	Concurrent    bool
}

func defaultTaskConfig() TaskConfig {
	return TaskConfig{
		Timeout:       0,
		MaxRetries:    0,
		RecoverPanics: false,
		Concurrent:    false,
	}
}
