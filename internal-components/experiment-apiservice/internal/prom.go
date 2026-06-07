package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

// defaultMetrics is the metric-family set captured by prom-snapshot (the
// headline observability-v2 set). One parquet per family is written.
var defaultMetrics = []string{
	"yass_file_produced_total",
	"yass_file_produced_bytes_total",
	"yass_file_received_total",
	"yass_file_received_bytes_total",
	"yass_file_lost_total",
	"yass_file_delivery_seconds_bucket",
	"yass_file_delivery_seconds_count",
	"yass_file_delivery_seconds_sum",
	"yass_battery_wh",
	"yass_battery_capacity_wh",
	"yass_battery_consumed_wh_total",
	"yass_in_shadow",
	"yass_low_power",
	"yass_volume_used_bytes",
	"yass_volume_capacity_bytes",
	"yass_container_cpu_millicores",
	"yass_container_memory_bytes",
	"yass_network_tx_bytes_total",
	"yass_network_rx_bytes_total",
	"yass_hardware_event_active",
	"yass_hardware_event_dropped_total",
	"yass_los_active",
	"yass_edfs_pin_intent_count",
	"yass_edfs_replica_count",
}

// promClient is a minimal Prometheus query_range client, ported from
// yass-compare's prom package.
type promClient struct {
	url  string
	http *http.Client
}

func newPromClient(promURL string) *promClient {
	if promURL == "" {
		return nil
	}
	return &promClient{url: promURL, http: &http.Client{Timeout: 30 * time.Second}}
}

type promSeries struct {
	metric  map[string]string
	samples []struct {
		t time.Time
		v float64
	}
}

func (c *promClient) queryRange(ctx context.Context, expr string, start, end time.Time, step time.Duration) ([]promSeries, error) {
	q := url.Values{}
	q.Set("query", expr)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.Itoa(int(step.Seconds()))+"s")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var r struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][]any           `json:"values"`
			} `json:"result"`
		} `json:"data"`
		ErrorType string `json:"errorType"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if r.Status != "success" {
		return nil, fmt.Errorf("prometheus error %s: %s", r.ErrorType, r.Error)
	}
	out := make([]promSeries, 0, len(r.Data.Result))
	for _, raw := range r.Data.Result {
		s := promSeries{metric: raw.Metric}
		for _, pair := range raw.Values {
			if len(pair) != 2 {
				continue
			}
			tsF, ok := pair[0].(float64)
			if !ok {
				continue
			}
			valS, _ := pair[1].(string)
			v, err := strconv.ParseFloat(valS, 64)
			if err != nil {
				continue // skip NaN/Inf
			}
			s.samples = append(s.samples, struct {
				t time.Time
				v float64
			}{t: time.Unix(int64(tsF), 0), v: v})
		}
		out = append(out, s)
	}
	return out, nil
}

// promMetricWide queries one metric family for the experiment-run and renders
// it wide: string label columns + one optional-float column per ISO-timestamp.
// The `max without (instance, pod, peer)` wrapper folds exporter-duplicate
// copies, matching prom-snapshot.
func (c *promClient) promMetricWide(ctx context.Context, experiment, runID, metric string, from, to time.Time, step time.Duration) (labelCols, tsCols []string, rows []wideMetricRow, err error) {
	expr := fmt.Sprintf("max without (instance, pod, peer) (%s{experiment=%q,run_id=%q})", metric, experiment, runID)
	series, err := c.queryRange(ctx, expr, from, to, step)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(series) == 0 {
		return nil, nil, nil, nil
	}

	labelSet := map[string]struct{}{}
	tsSet := map[time.Time]struct{}{}
	for _, s := range series {
		for k := range s.metric {
			labelSet[k] = struct{}{}
		}
		for _, sm := range s.samples {
			tsSet[sm.t] = struct{}{}
		}
	}
	labelCols = make([]string, 0, len(labelSet))
	for k := range labelSet {
		labelCols = append(labelCols, k)
	}
	sort.Strings(labelCols)

	tsList := make([]time.Time, 0, len(tsSet))
	for t := range tsSet {
		tsList = append(tsList, t)
	}
	sort.Slice(tsList, func(i, j int) bool { return tsList[i].Before(tsList[j]) })
	tsCols = make([]string, len(tsList))
	for i, t := range tsList {
		tsCols[i] = t.UTC().Format(time.RFC3339)
	}

	rows = make([]wideMetricRow, 0, len(series))
	for _, s := range series {
		wr := wideMetricRow{labels: make(map[string]string, len(labelCols)), values: make(map[string]float64, len(s.samples))}
		for _, k := range labelCols {
			wr.labels[k] = s.metric[k]
		}
		for _, sm := range s.samples {
			wr.values[sm.t.UTC().Format(time.RFC3339)] = sm.v
		}
		rows = append(rows, wr)
	}
	return labelCols, tsCols, rows, nil
}
