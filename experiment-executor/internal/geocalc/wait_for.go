package geocalc

import (
	"context"
	"os"
	"time"
)

func WaitForFile(ctx context.Context, path string) error {
	t := time.NewTicker(100 * time.Second)
	defer t.Stop()
	if fileExists(path) {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if fileExists(path) {
				return nil
			}
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return true
}
