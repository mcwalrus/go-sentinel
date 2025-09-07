package viewprom

import (
	"context"
	"errors"
	"iter"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Config struct {
	Timeout    time.Duration
	MaxRetries int
	Concurrent bool
}

func defaultConfig() Config {
	return Config{
		Timeout:    0,
		MaxRetries: 0,
		Concurrent: false,
	}
}

type ObserverConfig struct {
	Namespace     string
	Subsystem     string
	MetricsPrefix string
	Description   string
	Buckets       []float64
}

func (c ObserverConfig) isZero() bool {
	if c.Namespace+c.Subsystem+c.MetricsPrefix+c.Description == "" {
		if len(c.Buckets) == 0 {
			return true
		}
	}
	return false
}

func DefaultConfig() ObserverConfig {
	return ObserverConfig{
		Namespace:     "viewprom",
		Subsystem:     "",
		MetricsPrefix: "observer",
		Buckets:       []float64{0.01, 0.1, 1, 10, 100, 1000, 10_000},
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

func (o *Observer) Do(cfg Config, fn func(ctx context.Context) error) {
	task := &implTask{
		cfg: cfg,
		fn:  fn,
	}
	if !task.Config().Concurrent {
		o.observe(task)
	} else {
		go o.observe(task)
	}
}

func (o *Observer) Observe(fn func() error) {
	task := &implTask{
		cfg: defaultConfig(),
		fn: func(ctx context.Context) error {
			return fn() // ignore ctx
		},
	}
	o.observe(task)
}

func (o *Observer) ObserveTask(task Task) {
	if !task.Config().Concurrent {
		o.observe(task)
	} else {
		go o.observe(task)
	}
}

func (o *Observer) ObserveIter(tasks iter.Seq[Task]) {
	go func() {
		for task := range tasks {
			o.ObserveTask(task)
		}
	}()
}

func (o *Observer) Register(registry *prometheus.Registry) {
	o.metrics.Register(registry)
}

func (o *Observer) observe(task Task) {
	defer func() {
		if r := recover(); r != nil {
			o.metrics.Panics.Inc()
			panic(r)
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
			cfg.MaxRetries--

			retryTask := &implTask{
				fn:  task.Execute,
				cfg: cfg,
			}
			if !task.Config().Concurrent {
				completeTask()
				o.ObserveTask(retryTask)
			} else {
				go o.ObserveTask(retryTask)
			}
		}

	} else {
		o.metrics.Successes.Inc()
	}
}
