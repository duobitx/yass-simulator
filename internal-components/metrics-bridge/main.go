package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/go-common/startup"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/bridge"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/config"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/lokipush"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/metrics"
	"github.com/m-szalik/com-facade/mqtt"
	"github.com/m-szalik/goutils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const appName = "metrics-bridge"

func main() {
	goutils.ExitOnErrorf(startup.InitSlog(), 1, "cannot initialize slog")
	slog.Info("Metrics Bridge starting")

	cfg, err := config.FromEnv()
	goutils.ExitOnError(err, 2)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	m := metrics.New(reg)

	lp := lokipush.New(cfg.LokiURL, cfg.LokiTenant)
	go lp.Run(ctx)

	br := bridge.New(cfg, m, lp)
	hostname, _ := os.Hostname()
	clientID := fmt.Sprintf("%s-%s", appName, hostname)
	facade := mqtt.NewFacade(ctx, clientID, mqtt.WithHostPort("tcp://"+cfg.BrokerHostPort))
	goutils.ExitOnError(facade.Connect(), 3)
	goutils.ExitOnError(startup.FileProbe(ctx), 6)

	// One subscription per topic-prefix the bridge cares about. Wildcards
	// keep MQTT routing cheap on the broker side.
	for _, topic := range []string{"crud-events", "+/resources", "total-network-stats/+", "online-states/+", "experiment-lifecycle", "hardware-events/+", "updates/_time_"} {
		if err := facade.Subscribe(topic, br.Handle); err != nil {
			goutils.ExitOnError(fmt.Errorf("subscribe %s: %w", topic, err), 4)
		}
	}

	go runSweeper(ctx, br)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	server := &http.Server{Addr: cfg.ListenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("HTTP listening", "addr", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			cancel()
		}
	}()

	slog.Info("StartupCompleted",
		"experiment", cfg.ExperimentName,
		"engine", cfg.Engine,
		"run_id", cfg.RunID,
		"namespace", cfg.Namespace,
		"target_gs_by_fsnode", cfg.TargetGSByFsNode,
	)

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	slog.Info("Terminated")
}

func runSweeper(ctx context.Context, br *bridge.Bridge) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			br.Sweep(now)
		}
	}
}
