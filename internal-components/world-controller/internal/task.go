package internal

import (
	"context"
	"time"
)

func BackgroundPeriodicTask(ctx context.Context, period time.Duration, f func()) {
	ticker := time.NewTicker(period)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				f()
			}
		}
	}()
}
