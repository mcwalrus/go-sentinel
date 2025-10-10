package sentinel

import (
	"context"
)

type implTask struct {
	fn         func(ctx context.Context) error
	retryCount int
}
