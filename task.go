package sentinel

import (
	"context"
	_ "errors"
)

type implTask struct {
	fn         func(ctx context.Context) error
	cfg        ObserverConfig
	retryCount int
}
