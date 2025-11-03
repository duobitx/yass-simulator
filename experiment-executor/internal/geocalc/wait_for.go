package geocalc

import (
	"context"
	"fmt"
	"os"
	"time"
)

func WaitForFile(ctx context.Context, path string) error {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	if fileExists(path) {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("error waiting for file %s: %w", path, ctx.Err())
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
