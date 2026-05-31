package startup

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/m-szalik/goutils"
)

type httpHandler struct {
}

func (h httpHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.WriteHeader(http.StatusOK)
}

func HttpProbe(ctx context.Context, port int) {
	srv := &http.Server{
		Handler:           &httpHandler{},
		Addr:              fmt.Sprintf(":%d", port),
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
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			goutils.ExitOnErrorf(err, 80, "cannot start http server on port %d", port)
		}
	}()
}
