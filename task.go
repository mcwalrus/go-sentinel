package sentinel

import (
	"context"
)

type implTask struct {
	fn         func(ctx context.Context) error
	cfg        ObserverConfig
	retryCount int
}
