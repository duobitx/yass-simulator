package internal

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// eventKinds is the Loki `kind` label set produced by the engines (see the
// experiment data contract). One events/<kind>.parquet is written per kind.
var eventKinds = []string{"lifecycle", "online_state", "power", "crud", "file_delivered", "block_recv", "hardware"}

const resultsStep = 15 * time.Second

// handleResults streams a zip of parquet files for the experiment — read live
// and filtered by experiment + run_id:
//
//	events/<kind>.parquet    from Loki
//	metrics/<metric>.parquet from Prometheus (wide)
//
// The zip is streamed straight to the response and only one source is held in
// memory at a time, so peak memory stays bounded by the largest single source
// rather than the whole dataset. Loki/Prometheus being unreachable degrades to
// fewer entries instead of failing.
func (b *Backend) handleResults(ctx context.Context, ns, name string, w http.ResponseWriter, _ *http.Request) {
	exp := &yassv1.Experiment{}
	if err := b.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, exp); err != nil {
		http.Error(w, "experiment not found: "+err.Error(), http.StatusNotFound)
		return
	}
	runID := deriveRunID(exp)

	now := time.Now()
	created := exp.CreationTimestamp.Time
	// Loki sample time follows the simulation clock, which may predate creation.
	lokiStart := created
	if st := exp.Spec.SimulationStartTime; st != nil && !st.IsZero() && st.Time.Before(lokiStart) {
		lokiStart = st.Time
	}
	lokiStart = lokiStart.Add(-5 * time.Minute)
	lokiEnd := now.Add(time.Minute)
	// Prometheus stores wall-clock scrape time.
	promFrom := created.Add(-5 * time.Minute)
	promTo := now

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+"-"+runID+"-results.zip"))
	zw := zip.NewWriter(w)
	defer func() { _ = zw.Close() }()

	if b.loki != nil {
		for _, kind := range eventKinds {
			cols, rows, err := b.loki.lokiEventTable(ctx, name, runID, kind, lokiStart, lokiEnd)
			if err != nil {
				slog.Warn("results: loki kind failed (skipping)", "experiment", name, "kind", kind, "error", err)
				continue
			}
			if len(rows) == 0 {
				continue
			}
			if err := zipParquet(zw, "events/"+kind+".parquet", func(out io.Writer) error {
				return writeStringParquet(out, cols, rows)
			}); err != nil {
				slog.Warn("results: write events parquet failed", "kind", kind, "error", err)
				return
			}
		}
	}

	if b.prom != nil {
		for _, m := range defaultMetrics {
			labelCols, tsCols, rows, err := b.prom.promMetricWide(ctx, name, runID, m, promFrom, promTo, resultsStep)
			if err != nil {
				slog.Warn("results: prometheus metric failed (skipping)", "experiment", name, "metric", m, "error", err)
				continue
			}
			if len(rows) == 0 {
				continue
			}
			if err := zipParquet(zw, "metrics/"+m+".parquet", func(out io.Writer) error {
				return writeMetricWideParquet(out, labelCols, tsCols, rows)
			}); err != nil {
				slog.Warn("results: write metric parquet failed", "metric", m, "error", err)
				return
			}
		}
	}
}

// zipParquet adds one stored zip entry (no zip compression — parquet is already
// zstd-compressed) and writes the parquet into it.
func zipParquet(zw *zip.Writer, name string, write func(io.Writer) error) error {
	ew, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	return write(ew)
}

// deriveRunID mirrors the operator's run_id (metrics_bridge_mod.deriveRunID):
// Spec.RunID when set, else <name>_<creationTimestamp>. Must stay in sync.
func deriveRunID(exp *yassv1.Experiment) string {
	if exp.Spec.RunID != "" {
		return exp.Spec.RunID
	}
	return exp.Name + "_" + exp.CreationTimestamp.UTC().Format("20060102T150405Z")
}
