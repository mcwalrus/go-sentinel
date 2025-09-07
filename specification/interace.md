


# Service to provide

```Go
type Service interface {
    Init(conncurent, capacity int) 
    ObserveJob(job Job)
    ObserveSeq(queue iter.Seq[Job])
    Metrics() []prom.Metrics
}

type ServiceConfig struct {
    Conncurrent bool
    HttpEndpoint string
}

```


# User to implement

```Go
type Job interface {
    Config() JobConfig
	Execute(ctx context.Context) error
}

type JobConfig struct {
    Priority int,
    Timeout time.Duration,
    MaxRetries int,
    TrackMetrics bool,
    PromLabels map[string]string,
}
```