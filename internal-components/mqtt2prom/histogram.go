package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// histogramStore is a custom prometheus.Collector that exposes
// histogram metrics built from absolute bucket counts pushed in over
// MQTT. The standard HistogramVec / Observe API expects per-sample
// observations, not cumulative bucket counts, so we hand-roll a
// Collector that emits prometheus.NewConstHistogram per labelset
// during scrape.
//
// See yass-docs/observability-v2-spec.md §G1 (delivery_seconds p95)
// and §6 for the design.
type histogramStore struct {
	metricName string
	labelNames []string
	desc       *prometheus.Desc

	mu    sync.Mutex
	state map[string]histogramSnapshot
}

type histogramSnapshot struct {
	labelValues []string
	buckets     map[float64]uint64
	sum         float64
	count       uint64
}

func newHistogramStore(name string, labelNames []string) *histogramStore {
	return &histogramStore{
		metricName: name,
		labelNames: append([]string{}, labelNames...),
		desc:       prometheus.NewDesc(name, name+" (via mqtt2prom)", labelNames, nil),
		state:      map[string]histogramSnapshot{},
	}
}

func (h *histogramStore) Describe(ch chan<- *prometheus.Desc) {
	ch <- h.desc
}

func (h *histogramStore) Collect(ch chan<- prometheus.Metric) {
	h.mu.Lock()
	snaps := make([]histogramSnapshot, 0, len(h.state))
	for _, s := range h.state {
		snaps = append(snaps, s)
	}
	h.mu.Unlock()
	for _, s := range snaps {
		m, err := prometheus.NewConstHistogram(h.desc, s.count, s.sum, s.buckets, s.labelValues...)
		if err != nil {
			continue
		}
		ch <- m
	}
}

// Update stores the latest cumulative snapshot for a labelset. buckets
// is a map of upper-bound → cumulative count (as Prometheus expects in
// NewConstHistogram).
func (h *histogramStore) Update(labelValues []string, buckets map[float64]uint64, sum float64, count uint64) error {
	if len(labelValues) != len(h.labelNames) {
		return fmt.Errorf("histogram %s: label count mismatch want=%d got=%d", h.metricName, len(h.labelNames), len(labelValues))
	}
	key := strings.Join(labelValues, "\x00")
	bucketsCopy := make(map[float64]uint64, len(buckets))
	for k, v := range buckets {
		bucketsCopy[k] = v
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state[key] = histogramSnapshot{
		labelValues: append([]string{}, labelValues...),
		buckets:     bucketsCopy,
		sum:         sum,
		count:       count,
	}
	return nil
}

// Delete drops the snapshot for a labelset (called on aggregator
// eviction).
func (h *histogramStore) Delete(labelValues []string) {
	key := strings.Join(labelValues, "\x00")
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.state, key)
}
