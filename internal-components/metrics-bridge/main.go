package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/go-common/startup"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/bridge"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/config"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/k8sevents"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/lokipush"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/metrics"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/mqttpub"
	"github.com/m-szalik/com-facade/mqtt"
	"github.com/m-szalik/goutils"
	"github.com/prometheus/client_golang/prometheus"
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

	events, err := k8sevents.New(ctx, cfg.ExperimentName, cfg.Namespace, appName, cfg.K8sEventsSkipKinds)
	if err != nil {
		slog.Warn("k8s events disabled", "error", err)
	}

	br := bridge.New(cfg, m, lp, events)
	hostname, _ := os.Hostname()
	clientID := fmt.Sprintf("%s-%s", appName, hostname)
	facade := mqtt.NewFacade(ctx, clientID, mqtt.WithHostPort("tcp://"+cfg.BrokerHostPort))
	goutils.ExitOnError(facade.Connect(), 3)
	goutils.ExitOnError(startup.FileProbe(ctx), 6)

	// One subscription per topic-prefix the bridge cares about. Wildcards
	// keep MQTT routing cheap on the broker side.
	for _, topic := range []string{"crud-events", "+/resources", "total-network-stats/+", "online-states/+", "experiment-lifecycle", "hardware-events/+", "los/+", "edfs-cids/+", "updates/_time_"} {
		if err := facade.Subscribe(topic, br.Handle); err != nil {
			goutils.ExitOnError(fmt.Errorf("subscribe %s: %w", topic, err), 4)
		}
	}

	go runSweeper(ctx, br)

	// Phase E: publish metrics on MQTT instead of HTTP /metrics.
	// `metrics/_meta_/<ns>` retained once at startup; per-labelset
	// snapshots on `metrics/<name>/<hash>` every PublishInterval.
	// See yass-docs/observability-v2-spec.md §6.
	aggregator := cfg.Namespace
	if aggregator == "" {
		aggregator = cfg.ExperimentName
	}
	pub := mqttpub.New(facade, reg, mqttpub.Meta{
		Aggregator: aggregator,
		Experiment: cfg.ExperimentName,
		Engine:     cfg.Engine,
		RunID:      cfg.RunID,
		Layout:     cfg.Layout,
		Namespace:  cfg.Namespace,
	}, 5*time.Second)
	go pub.Run(ctx)

	slog.Info("StartupCompleted",
		"experiment", cfg.ExperimentName,
		"engine", cfg.Engine,
		"run_id", cfg.RunID,
		"layout", cfg.Layout,
		"namespace", cfg.Namespace,
		"aggregator", aggregator,
		"target_gs_by_fsnode", cfg.TargetGSByFsNode,
	)

	<-ctx.Done()
	// Best-effort graceful: clear retained _meta_ so mqtt2prom drops
	// our metrics promptly.
	clearCtx, clearCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer clearCancel()
	if err := pub.ClearMeta(clearCtx); err != nil {
		slog.Warn("ClearMeta failed", "error", err)
	}
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
