// Package mqttpub walks a prometheus.Gatherer periodically and
// publishes each metric family on `metrics/<name>/<labelset_hash>` as
// QoS 0 retained — replacing the v1 HTTP /metrics endpoint.
//
// See yass-docs/observability-v2-spec.md §6 for the contract and
// payload format.
package mqttpub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	com "github.com/m-szalik/com-facade"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const (
	topicMetricPrefix = "metrics/"
	topicMetaPrefix   = "metrics/_meta_/"
)

// Meta carries the experiment-identifying labels each aggregator
// advertises once on `metrics/_meta_/<aggregator>` (retained). mqtt2prom
// joins every metric snapshot to its meta record by the `aggregator`
// field on the snapshot.
type Meta struct {
	Aggregator string `json:"aggregator"`
	Experiment string `json:"experiment"`
	Engine     string `json:"engine"`
	RunID      string `json:"run_id"`
	Layout     string `json:"layout"`
	Namespace  string `json:"namespace"`
}

// Publisher walks gatherer every Interval and publishes each metric
// family on MQTT. ClearOnStop publishes empty retained _meta_ at
// shutdown so mqtt2prom drops the aggregator's metrics cleanly.
type Publisher struct {
	facade   com.Facade
	gatherer prometheus.Gatherer
	meta     Meta
	interval time.Duration
}

func New(facade com.Facade, gatherer prometheus.Gatherer, meta Meta, interval time.Duration) *Publisher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Publisher{facade: facade, gatherer: gatherer, meta: meta, interval: interval}
}

// PublishMeta publishes the retained `metrics/_meta_/<aggregator>`
// once. Call after the MQTT facade is connected.
func (p *Publisher) PublishMeta(ctx context.Context) error {
	body, err := json.Marshal(p.meta)
	if err != nil {
		return err
	}
	return p.facade.Publish(ctx, topicMetaPrefix+p.meta.Aggregator, 0, true, body)
}

// ClearMeta publishes an empty retained payload on the _meta_ topic
// so mqtt2prom drops everything joined to this aggregator. Call from
// SIGTERM / context cancellation handlers.
func (p *Publisher) ClearMeta(ctx context.Context) error {
	return p.facade.Publish(ctx, topicMetaPrefix+p.meta.Aggregator, 0, true, []byte{})
}

// Run blocks until ctx is cancelled, publishing snapshots every
// p.interval. On exit it does NOT clear meta — caller decides whether
// to call ClearMeta (graceful shutdown) or leave it (crash → stale
// metrics survive but will be replaced when a new aggregator binds
// the same name).
func (p *Publisher) Run(ctx context.Context) {
	if err := p.PublishMeta(ctx); err != nil {
		slog.Warn("mqttpub: meta publish failed", "error", err)
	}
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			p.snapshot(ctx, now)
		}
	}
}

func (p *Publisher) snapshot(ctx context.Context, now time.Time) {
	mfs, err := p.gatherer.Gather()
	if err != nil {
		slog.Warn("mqttpub: gather failed", "error", err)
		return
	}
	for _, mf := range mfs {
		name := mf.GetName()
		kind := metricKind(mf.GetType())
		for _, m := range mf.GetMetric() {
			payload := buildSnapshot(name, kind, p.meta.Aggregator, m, now)
			body, err := json.Marshal(payload)
			if err != nil {
				slog.Warn("mqttpub: marshal", "metric", name, "error", err)
				continue
			}
			topic := topicMetricPrefix + name + "/" + labelsetHash(m.GetLabel())
			if err := p.facade.Publish(ctx, topic, 0, true, body); err != nil {
				slog.Warn("mqttpub: publish", "topic", topic, "error", err)
			}
		}
	}
}

func metricKind(t dto.MetricType) string {
	switch t {
	case dto.MetricType_COUNTER:
		return "counter"
	case dto.MetricType_GAUGE:
		return "gauge"
	case dto.MetricType_HISTOGRAM:
		return "histogram"
	case dto.MetricType_SUMMARY:
		return "summary"
	default:
		return "untyped"
	}
}

type snapshot struct {
	Metric     string             `json:"metric"`
	Kind       string             `json:"kind"`
	Aggregator string             `json:"aggregator"`
	Labels     map[string]string  `json:"labels"`
	At         string             `json:"at"`
	Value      *float64           `json:"value,omitempty"`
	Sum        *float64           `json:"sum,omitempty"`
	Count      *uint64            `json:"count,omitempty"`
	Buckets    []bucketSnapshot   `json:"buckets,omitempty"`
}

type bucketSnapshot struct {
	UpperBound float64 `json:"le"`
	Count      uint64  `json:"count"`
}

func buildSnapshot(name, kind, aggregator string, m *dto.Metric, now time.Time) snapshot {
	s := snapshot{
		Metric:     name,
		Kind:       kind,
		Aggregator: aggregator,
		Labels:     labelMap(m.GetLabel()),
		At:         now.UTC().Format(time.RFC3339Nano),
	}
	switch {
	case m.Counter != nil:
		v := m.Counter.GetValue()
		s.Value = &v
	case m.Gauge != nil:
		v := m.Gauge.GetValue()
		s.Value = &v
	case m.Histogram != nil:
		sum := m.Histogram.GetSampleSum()
		cnt := m.Histogram.GetSampleCount()
		s.Sum, s.Count = &sum, &cnt
		for _, b := range m.Histogram.GetBucket() {
			s.Buckets = append(s.Buckets, bucketSnapshot{UpperBound: b.GetUpperBound(), Count: b.GetCumulativeCount()})
		}
	case m.Summary != nil:
		sum := m.Summary.GetSampleSum()
		cnt := m.Summary.GetSampleCount()
		s.Sum, s.Count = &sum, &cnt
	}
	return s
}

func labelMap(pairs []*dto.LabelPair) map[string]string {
	if len(pairs) == 0 {
		return nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		out[p.GetName()] = p.GetValue()
	}
	return out
}

// labelsetHash is a stable, short hash of the (label, value) pairs.
// Different orderings of the same pairs produce the same hash, so MQTT
// retained-state size is bounded by the *distinct* labelset count.
func labelsetHash(pairs []*dto.LabelPair) string {
	if len(pairs) == 0 {
		return "0"
	}
	keys := make([]string, 0, len(pairs))
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		keys = append(keys, p.GetName())
		m[p.GetName()] = p.GetValue()
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = h.Write([]byte(k))
		_, _ = h.Write([]byte{'='})
		_, _ = h.Write([]byte(m[k]))
		_, _ = h.Write([]byte{0})
	}
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8])
}

// TopicMetric returns the topic a snapshot of this (metric, labelset)
// will be published on. Exported for tests.
func TopicMetric(name string, labels []*dto.LabelPair) string {
	return fmt.Sprintf("%s%s/%s", topicMetricPrefix, name, labelsetHash(labels))
}

// TopicMeta returns the meta topic for an aggregator.
func TopicMeta(aggregator string) string { return topicMetaPrefix + aggregator }
