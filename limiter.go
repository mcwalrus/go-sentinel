package sentinel

type limiter chan struct{}

func (l limiter) acquire() <-chan struct{} {
	acquired := make(chan struct{})
	if l == nil {
		close(acquired)
		return acquired
	}

	select {
	case l <- struct{}{}:
		close(acquired)
		return acquired
	default:
		go func() {
			l <- struct{}{}
			close(acquired)
		}()
		return acquired
	}
}

func (l limiter) release() {
	if l != nil {
		<-l
	}
}
