package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestHandleMetaThenSnapshotRegistersWithJoinedLabels(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := newCollector(reg)
	c.handleMeta("spain-shot-tus", mustJSON(t, meta{
		Aggregator: "spain-shot-tus",
		Experiment: "spain-shot",
		Engine:     "tus",
		RunID:      "spain-shot_2026",
		Layout:     "spain-shot-layout",
		Namespace:  "spain-shot-tus",
	}))
	v := float64(42)
	c.handleSnapshot(mustJSON(t, snapshot{
		Metric:     "yass_battery_wh",
		Kind:       "gauge",
		Aggregator: "spain-shot-tus",
		Labels:     map[string]string{"fsNode": "oneweb-0008"},
		Value:      &v,
	}))
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() != "yass_battery_wh" {
			continue
		}
		found = true
		if len(mf.Metric) != 1 {
			t.Fatalf("want 1 metric, got %d", len(mf.Metric))
		}
		got := labelMapFromPairs(mf.Metric[0].Label)
		for _, want := range []string{"experiment=spain-shot", "engine=tus", "run_id=spain-shot_2026", "layout=spain-shot-layout", "namespace=spain-shot-tus", "fsNode=oneweb-0008"} {
			if !strings.Contains(got, want) {
				t.Errorf("want label %q in %q", want, got)
			}
		}
		if mf.Metric[0].GetGauge().GetValue() != 42 {
			t.Errorf("want value 42, got %v", mf.Metric[0].GetGauge().GetValue())
		}
	}
	if !found {
		t.Fatalf("yass_battery_wh not in registry")
	}
}

func TestSnapshotBeforeMetaIsIgnored(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := newCollector(reg)
	v := float64(1)
	c.handleSnapshot(mustJSON(t, snapshot{
		Metric:     "yass_x",
		Kind:       "gauge",
		Aggregator: "unknown",
		Value:      &v,
	}))
	mfs, _ := reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "yass_x" {
			t.Errorf("metric should not register without meta")
		}
	}
}

func TestEmptyMetaEvictsAggregatorSeries(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := newCollector(reg)
	c.handleMeta("agg", mustJSON(t, meta{Aggregator: "agg", Experiment: "exp", Engine: "tus"}))
	v := float64(5)
	c.handleSnapshot(mustJSON(t, snapshot{
		Metric: "yass_x", Kind: "gauge", Aggregator: "agg",
		Labels: map[string]string{"fsNode": "n"}, Value: &v,
	}))
	// sanity: series exists
	mfs, _ := reg.Gather()
	if !hasMetric(mfs, "yass_x") {
		t.Fatalf("yass_x not present after first snapshot")
	}
	c.handleMeta("agg", []byte{}) // empty payload → aggregator gone
	mfs, _ = reg.Gather()
	for _, mf := range mfs {
		if mf.GetName() == "yass_x" && len(mf.Metric) > 0 {
			t.Errorf("yass_x series should be evicted; got %d series", len(mf.Metric))
		}
	}
}

func hasMetric(mfs []*dto.MetricFamily, name string) bool {
	for _, mf := range mfs {
		if mf.GetName() == name && len(mf.Metric) > 0 {
			return true
		}
	}
	return false
}

func labelMapFromPairs(pairs []*dto.LabelPair) string {
	parts := []string{}
	for _, p := range pairs {
		parts = append(parts, p.GetName()+"="+p.GetValue())
	}
	return strings.Join(parts, ",")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	return b
}
