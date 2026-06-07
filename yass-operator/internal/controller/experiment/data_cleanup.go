package experiment

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
)

// dataDeleteTimeout bounds each observability delete call so an absent or hung
// Loki/Prometheus can never block the experiment's deletion.
const dataDeleteTimeout = 5 * time.Second

// deleteExperimentData best-effort removes this experiment's data from Loki and
// Prometheus, filtered by experiment name AND run_id. It NEVER returns an error
// and never blocks for long: a missing or unreachable Loki/Prometheus must not
// crash or stall the operator. All failures are logged and swallowed so the
// experiment can still be deleted.
func (r *Reconciler) deleteExperimentData(ctx context.Context, experiment *yassv1.Experiment) {
	runID := deriveRunID(experiment)
	// LogQL / PromQL stream selector matching this experiment's series only.
	selector := fmt.Sprintf(`{experiment=%q, run_id=%q}`, experiment.Name, runID)
	r.deleteLokiData(ctx, experiment, selector)
	r.deletePrometheusData(ctx, experiment.Name, selector)
}

func (r *Reconciler) deleteLokiData(ctx context.Context, experiment *yassv1.Experiment, selector string) {
	base := strings.TrimRight(r.Configuration.ObservabilityLokiURL, "/")
	if base == "" {
		return
	}
	// Bracket the experiment's data window. The Loki sample timestamp follows the
	// simulation clock, which may predate the wall-clock creation time.
	ref := experiment.CreationTimestamp.Time
	if st := experiment.Spec.SimulationStartTime; st != nil && !st.IsZero() {
		ref = st.Time
	}
	q := url.Values{}
	q.Set("query", selector)
	q.Set("start", fmt.Sprintf("%d", ref.Add(-time.Hour).Unix()))
	q.Set("end", fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()))

	status, err := r.postObservabilityDelete(ctx, base+"/loki/api/v1/delete?"+q.Encode(), r.Configuration.ObservabilityLokiTenant)
	if err != nil {
		slog.Default().Warn("loki data delete skipped (unreachable)", "experiment", experiment.Name, "error", err)
		return
	}
	if status >= 300 {
		slog.Default().Warn("loki data delete not accepted", "experiment", experiment.Name, "status", status)
		return
	}
	slog.Default().Info("loki data delete requested", "experiment", experiment.Name, "selector", selector)
}

func (r *Reconciler) deletePrometheusData(ctx context.Context, experimentName, selector string) {
	base := strings.TrimRight(r.Configuration.ObservabilityPrometheusURL, "/")
	if base == "" {
		return
	}
	// No start/end -> delete every matching series across all time. Requires
	// Prometheus to run with --web.enable-admin-api (otherwise 4xx, swallowed).
	q := url.Values{}
	q.Set("match[]", selector)

	status, err := r.postObservabilityDelete(ctx, base+"/api/v1/admin/tsdb/delete_series?"+q.Encode(), "")
	if err != nil {
		slog.Default().Warn("prometheus data delete skipped (unreachable)", "experiment", experimentName, "error", err)
		return
	}
	if status >= 300 {
		slog.Default().Warn("prometheus data delete not accepted (admin API enabled?)", "experiment", experimentName, "status", status)
		return
	}
	slog.Default().Info("prometheus data delete requested", "experiment", experimentName, "selector", selector)
}

// postObservabilityDelete issues a short-lived POST and returns the status code.
func (r *Reconciler) postObservabilityDelete(ctx context.Context, fullURL, lokiTenant string) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, dataDeleteTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, fullURL, nil)
	if err != nil {
		return 0, err
	}
	if lokiTenant != "" {
		req.Header.Set("X-Scope-OrgID", lokiTenant)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer goutils.CloseQuietly(resp.Body)
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}
