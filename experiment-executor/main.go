package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/internal"
	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/internal/geocalc"
	"github.com/gorilla/mux"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/ESA-PhiLab/yass-internal-components/experiment-executor/consts"
	"github.com/ESA-PhiLab/yass-internal-components/go-common/com"
	"github.com/m-szalik/goutils"
)

func main() {
	experiment := goutils.EnvRequired[string]("YASS_EXPERIMENT")
	slog.Info("ExperimentExecutor", "experiment", experiment)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	facade := com.NewFacade(ctx, fmt.Sprintf("%s-%d", consts.AppName, rand.Int()))
	err := facade.Connect()
	goutils.ExitOnError(err, 2)
	app, err := internal.NewApp(ctx, facade)
	goutils.ExitOnError(err, 3)

	router := mux.NewRouter()
	app.DefineEndpoints(router)
	srv := &http.Server{
		Handler:           router,
		Addr:              "0.0.0.0:8080",
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("HTTP server starting", "addr", srv.Addr)
	go func() {
		<-ctx.Done()
		slog.Info("Shutdown signal received, shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", "error", err)
		}
	}()

	slog.Info("StartupCompleted....")

	err = app.Start()             // FIXME
	goutils.ExitOnError(err, 111) // FIXME mock

	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("HTTP server stopped unexpectedly", "error", err)
	} else {
		slog.Info("HTTP server stopped")
	}
	slog.Info("Terminated")
}
