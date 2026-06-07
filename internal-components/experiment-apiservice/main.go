package main

import (
	"context"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/duobitx/yass-simulator/internal-components/experiment-apiservice/internal"
	"github.com/duobitx/yass-simulator/internal-components/go-common/startup"
	"github.com/m-szalik/goutils"
)

func main() {
	goutils.ExitOnErrorf(startup.InitSlog(), 1, "cannot initialize slog")
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	opts := internal.OptionsFromEnv()
	slog.Info("experiment-apiservice starting",
		"group", internal.APIGroup, "version", internal.APIVersion, "securePort", opts.SecurePort)
	goutils.ExitOnError(internal.Run(ctx, opts), 2)
	slog.Info("Terminated")
}
