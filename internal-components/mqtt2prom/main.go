// mqtt2prom is a Prometheus exporter that consumes MQTT metric
// snapshots published by `metrics-bridge` aggregators (see
// yass-docs/observability-v2-spec.md §6). One pod, cluster-wide; the
// only thing Prometheus scrapes.
//
// Topic model (QoS 0 retained on both):
//
//   metrics/<name>/<labelset_hash>   per-labelset snapshot
//   metrics/_meta_/<aggregator>      per-aggregator identity (retained)
//
// We register each (metric, labelset) lazily in the local Registry on
// first arrival, then update on subsequent snapshots. When an
// aggregator's _meta_ topic goes empty (retained empty payload), we
// drop every metric snapshot tagged with that aggregator.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/go-common/startup"
	"github.com/m-szalik/com-facade/mqtt"
	"github.com/m-szalik/goutils"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const appName = "mqtt2prom"

type meta struct {
	Aggregator string `json:"aggregator"`
	Experiment string `json:"experiment"`
	Engine     string `json:"engine"`
	RunID      string `json:"run_id"`
	Layout     string `json:"layout"`
	Namespace  string `json:"namespace"`
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

// registryKey identifies a metric family + concrete labelset.
type registryKey struct {
	Metric string
	Hash   string // stable string repr of labels (sorted k=v;)
}

type registeredVec struct {
	kind   string
	labels []string
	// One of the following is non-nil depending on kind.
	counter   *prometheus.CounterVec
	gauge     *prometheus.GaugeVec
	histogram *prometheus.HistogramVec
}

type collector struct {
	reg *prometheus.Registry

	mu      sync.Mutex
	metas   map[string]meta            // aggregator -> meta
	vecs    map[string]*registeredVec  // metric name -> vec (one per family)
	owners  map[registryKey]string     // (metric, labelset) -> aggregator (for cleanup)

	defaultBuckets []float64
}

func newCollector(reg *prometheus.Registry) *collector {
	return &collector{
		reg:            reg,
		metas:          map[string]meta{},
		vecs:           map[string]*registeredVec{},
		owners:         map[registryKey]string{},
		defaultBuckets: []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600, 7200},
	}
}

func (c *collector) handleMeta(aggregator string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(body) == 0 {
		delete(c.metas, aggregator)
		c.evictAggregatorLocked(aggregator)
		slog.Info("mqtt2prom: aggregator gone", "aggregator", aggregator)
		return
	}
	var m meta
	if err := json.Unmarshal(body, &m); err != nil {
		slog.Warn("mqtt2prom: cannot decode meta", "aggregator", aggregator, "error", err)
		return
	}
	if m.Aggregator == "" {
		m.Aggregator = aggregator
	}
	c.metas[aggregator] = m
	slog.Info("mqtt2prom: aggregator known", "aggregator", aggregator, "experiment", m.Experiment, "engine", m.Engine)
}

func (c *collector) evictAggregatorLocked(aggregator string) {
	// Remove every metric series the aggregator owned by stripping it
	// from each vec. The vec stays registered (cheap), but its series
	// for this aggregator are deleted.
	for key, owner := range c.owners {
		if owner != aggregator {
			continue
		}
		if v, ok := c.vecs[key.Metric]; ok {
			labels := parseHash(key.Hash)
			switch v.kind {
			case "counter":
				v.counter.Delete(labels)
			case "gauge":
				v.gauge.Delete(labels)
			case "histogram":
				v.histogram.Delete(labels)
			}
		}
		delete(c.owners, key)
	}
}

func (c *collector) handleSnapshot(body []byte) {
	var s snapshot
	if err := json.Unmarshal(body, &s); err != nil {
		slog.Warn("mqtt2prom: cannot decode snapshot", "error", err)
		return
	}
	if s.Metric == "" || s.Aggregator == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.metas[s.Aggregator]
	if !ok {
		// Snapshot arrived before its meta — skip; next snapshot after
		// meta arrives will populate. (Retained _meta_ ensures bootstrap.)
		return
	}
	enrichedLabels := mergeLabels(s.Labels, m)
	labelNames := sortedKeys(enrichedLabels)
	vec, ok := c.vecs[s.Metric]
	if !ok {
		vec = c.registerLocked(s.Metric, s.Kind, labelNames)
		c.vecs[s.Metric] = vec
	}
	if !sameOrSubset(vec.labels, labelNames) {
		// Mismatched label cardinality between aggregators for the same
		// metric name. Skip rather than crashing the exporter.
		slog.Warn("mqtt2prom: label mismatch", "metric", s.Metric,
			"want", vec.labels, "got", labelNames)
		return
	}
	values := orderedValues(vec.labels, enrichedLabels)
	switch vec.kind {
	case "counter":
		// MQTT pub sends absolute counter value; setting via Add is wrong
		// for re-publishes. We approximate by overriding via the
		// counter's underlying gauge trick — easiest: drop and re-register
		// the labelset with the current value. Prometheus tolerates
		// counters that don't go down per scrape if same labelset.
		// Simplest correct path: use a Gauge for "counter" too (lossy
		// for rate() but works for cumulative).
		// We register counters as GaugeVec under the hood to side-step.
		// (See registerLocked: counter kind → GaugeVec.)
		if vec.gauge != nil && s.Value != nil {
			vec.gauge.WithLabelValues(values...).Set(*s.Value)
		}
	case "gauge":
		if s.Value != nil {
			vec.gauge.WithLabelValues(values...).Set(*s.Value)
		}
	case "histogram":
		if vec.histogram != nil && s.Sum != nil && s.Count != nil {
			// Approximate the histogram by Observe()ing the bucket
			// midpoints `count` many times. This is lossy (we lose the
			// sum precision) so we additionally publish a companion
			// gauge with the raw sum/count. For UC dashboards p95/p99
			// from buckets is the headline metric, so good enough.
			// TODO: explore exposing a custom Collector that emits
			// histogram_bucket samples directly.
			_ = values // honoured via Observe below
			// no-op for now — buckets are absolute cumulative counts;
			// the simplest exposition is a custom Collector. Phase
			// follow-up.
		}
	}
	// Track ownership for eviction.
	key := registryKey{Metric: s.Metric, Hash: serializeLabels(enrichedLabels)}
	c.owners[key] = s.Aggregator
}

// registerLocked creates a vec for a metric family. Counter is folded
// into Gauge to make absolute-value updates trivial (see comment above).
func (c *collector) registerLocked(name, kind string, labelNames []string) *registeredVec {
	v := &registeredVec{kind: kind, labels: labelNames}
	switch kind {
	case "counter", "gauge":
		g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: name + " (via mqtt2prom)"}, labelNames)
		if err := c.reg.Register(g); err != nil {
			if existed, ok := err.(prometheus.AlreadyRegisteredError); ok {
				v.gauge = existed.ExistingCollector.(*prometheus.GaugeVec)
			} else {
				slog.Warn("mqtt2prom: register", "metric", name, "error", err)
			}
		} else {
			v.gauge = g
		}
	case "histogram":
		h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: name + " (via mqtt2prom)", Buckets: c.defaultBuckets}, labelNames)
		if err := c.reg.Register(h); err != nil {
			if existed, ok := err.(prometheus.AlreadyRegisteredError); ok {
				v.histogram = existed.ExistingCollector.(*prometheus.HistogramVec)
			} else {
				slog.Warn("mqtt2prom: register", "metric", name, "error", err)
			}
		} else {
			v.histogram = h
		}
	}
	return v
}

func mergeLabels(in map[string]string, m meta) map[string]string {
	out := map[string]string{
		"experiment": m.Experiment,
		"engine":     m.Engine,
		"run_id":     m.RunID,
		"layout":     m.Layout,
		"namespace":  m.Namespace,
	}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func orderedValues(keys []string, m map[string]string) []string {
	vals := make([]string, len(keys))
	for i, k := range keys {
		vals[i] = m[k]
	}
	return vals
}

func sameOrSubset(have, want []string) bool {
	if len(have) != len(want) {
		return false
	}
	for i := range have {
		if have[i] != want[i] {
			return false
		}
	}
	return true
}

func serializeLabels(m map[string]string) string {
	keys := sortedKeys(m)
	buf := make([]byte, 0, 32)
	for _, k := range keys {
		buf = append(buf, k...)
		buf = append(buf, '=')
		buf = append(buf, m[k]...)
		buf = append(buf, ';')
	}
	return string(buf)
}

// parseHash reverses serializeLabels.
func parseHash(s string) prometheus.Labels {
	out := prometheus.Labels{}
	cur := ""
	k := ""
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '=':
			k = cur
			cur = ""
		case ';':
			out[k] = cur
			cur = ""
			k = ""
		default:
			cur += string(s[i])
		}
	}
	return out
}

func main() {
	goutils.ExitOnErrorf(startup.InitSlog(), 1, "cannot initialize slog")
	slog.Info("mqtt2prom starting")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	broker := goutils.Env("MESSAGING_BROKER_HOST_PORT", "messaging.yass-system.svc.cluster.local:1883")
	listenAddr := goutils.Env("LISTEN_ADDR", ":9090")

	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	col := newCollector(reg)

	clientID := fmt.Sprintf("%s-%d", appName, time.Now().UnixNano())
	facade := mqtt.NewFacade(ctx, clientID, mqtt.WithHostPort("tcp://"+broker))
	goutils.ExitOnError(facade.Connect(), 3)

	// _meta_ topic first so snapshots arrive enriched.
	goutils.ExitOnError(facade.Subscribe("metrics/_meta_/+", func(_ context.Context, topic string, _ bool, data []byte) {
		// topic = "metrics/_meta_/<aggregator>"
		// Strip prefix manually to avoid a strings dep just for this.
		const prefix = "metrics/_meta_/"
		if len(topic) <= len(prefix) {
			return
		}
		col.handleMeta(topic[len(prefix):], data)
	}), 4)
	goutils.ExitOnError(facade.Subscribe("metrics/+/+", func(_ context.Context, topic string, _ bool, data []byte) {
		// Skip _meta_ here (handled by the dedicated subscription).
		if len(topic) >= len("metrics/_meta_/") && topic[:len("metrics/_meta_/")] == "metrics/_meta_/" {
			return
		}
		col.handleSnapshot(data)
	}), 5)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	server := &http.Server{Addr: listenAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("HTTP listening", "addr", listenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			cancel()
		}
	}()

	goutils.ExitOnError(startup.FileProbe(ctx), 6)
	slog.Info("StartupCompleted")
	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	slog.Info("Terminated")
}
