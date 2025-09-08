package sentinel

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type ObserverConfig struct {
	Namespace   string
	Subsystem   string
	Description string
	Buckets     []float64
}

func (c ObserverConfig) isZero() bool {
	if c.Namespace+c.Subsystem+c.Description == "" {
		if len(c.Buckets) == 0 {
			return true
		}
	}
	return false
}

func DefaultConfig() ObserverConfig {
	return ObserverConfig{
		Namespace: "viewprom",
		Subsystem: "observer",
		Buckets:   []float64{0.01, 0.1, 1, 10, 100, 1000, 10_000, 100_000},
	}
}

type Observer struct {
	cfg     ObserverConfig
	metrics *metrics
}

func NewObserver(cfg ObserverConfig) *Observer {
	if cfg.isZero() {
		cfg = DefaultConfig()
	}
	return &Observer{
		cfg:     cfg,
		metrics: newMetrics(cfg),
	}
}

func (o *Observer) Register(registry *prometheus.Registry) {
	o.metrics.Register(registry)
}

func (o *Observer) MustRegister(registry *prometheus.Registry) {
	o.metrics.MustRegister(registry)
}

func (o *Observer) Run(cfg TaskConfig, fn func(ctx context.Context) error) error {
	task := &implTask{
		cfg: cfg,
		fn:  fn,
	}
	if !task.Config().Concurrent {
		return o.observe(task)
	} else {
		go o.observe(task)
	}
	return nil
}

func (o *Observer) RunFunc(fn func() error) error {
	task := &implTask{
		cfg: defaultTaskConfig(),
		fn: func(ctx context.Context) error {
			return fn() // ignore ctx
		},
	}
	if !task.Config().Concurrent {
		return o.observe(task)
	} else {
		go o.observe(task)
	}
	return nil
}

func (o *Observer) RunTask(task Task) error {
	t := &implTask{
		cfg: task.Config(),
		fn:  task.Execute,
	}
	if !task.Config().Concurrent {
		return o.observe(t)
	} else {
		go o.observe(t)
	}
	return nil
}

func (o *Observer) observe(task *implTask) error {
	defer func() {
		if r := recover(); r != nil {
			o.metrics.Panics.Inc()
			if !task.Config().RecoverPanics {
				panic(r)
			}
		}
	}()

	start := time.Now()
	o.metrics.InFlight.Inc()
	observeOnce := sync.Once{}

	completeTask := func() {
		observeOnce.Do(func() {
			o.metrics.InFlight.Dec()
			o.metrics.ObservedRuntimes.Observe(
				time.Since(start).Seconds(),
			)
		})
	}
	defer completeTask()

	ctx := context.Background()
	if task.Config().Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, task.Config().Timeout)
		defer cancel()
	}

	err := task.Execute(ctx)
	if err != nil {
		o.metrics.Errors.Inc()
		if errors.Is(err, context.DeadlineExceeded) {
			o.metrics.TimeoutErrors.Inc()
		}

		if task.Config().MaxRetries > 0 {
			o.metrics.Retries.Inc()
			cfg := task.Config()
			task.retryCount++
			completeTask()

			retryAttempt := cfg.MaxRetries - task.retryCount
			wait := cfg.RetryStrategy(retryAttempt)
			time.Sleep(wait)

			retryTask := &implTask{
				fn:         task.Execute,
				cfg:        cfg,
				retryCount: task.retryCount,
			}

			if !retryTask.Config().Concurrent {
				err2 := o.observe(retryTask)
				if err2 != nil {
					return errors.Join(err, err2)
				} else {
					return nil
				}
			} else {
				go o.observe(retryTask)
			}
		}
	} else {
		o.metrics.Successes.Inc()
	}

	return err
}
