package sentinel

import (
	"context"
	"time"
)

type Task interface {
	Config() TaskConfig
	Execute(ctx context.Context) error
}

type TaskConfig struct {
	Timeout       time.Duration
	Concurrent    bool
	MaxRetries    int
	RecoverPanics bool
	RetryStrategy func(retryCount int) time.Duration
}

func defaultTaskConfig() TaskConfig {
	return TaskConfig{
		Timeout:       0,
		MaxRetries:    0,
		RecoverPanics: false,
		Concurrent:    false,
		RetryStrategy: RetryStrategyImmediate,
	}
}

type implTask struct {
	fn         func(ctx context.Context) error
	cfg        TaskConfig
	retryCount int
}

func (t *implTask) Config() TaskConfig {
	return t.cfg
}

func (t *implTask) Execute(ctx context.Context) error {
	return t.fn(ctx)
}
