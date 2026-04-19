package startup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/pkg/errors"
)

func FileProbe(ctx context.Context) error {
	file := os.Getenv("FILE_PROBE_PATH")
	if file == "" {
		file = "/tmp/probe.txt"
	}
	t := time.Now().UTC().Format(time.RFC3339)
	err := os.WriteFile(file, []byte(t), 0o777)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("cannot wite file probe to %s", file))
	}
	slog.Default().Info("Probe file created", "file", file)
	go func() {
		<-ctx.Done()
		err := os.Remove(file)
		if err != nil {
			slog.Default().Warn("cannot remove prob file", "file", file, "error", err)
		} else {
			slog.Default().Info("Probe file removed", "file", file)
		}
	}()
	return nil
}
