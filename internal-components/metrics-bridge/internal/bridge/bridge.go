package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/config"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/metrics"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/state"
	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	"github.com/prometheus/client_golang/prometheus"
)

// crudEvent mirrors fs-engines/fs_engine_wrapper/pkg/notifier.NotifyEvent
// minus the dep cycle; the bridge only needs the JSON fields.
type crudEvent struct {
	Name             string         `json:"Name"`
	ContentSizeBytes int64          `json:"ContentSizeBytes"`
	Attributes       map[string]int `json:"Attributes"`
	FsNodeName       string         `json:"FsNodeName"`
	When             time.Time      `json:"When"`
	Type             string         `json:"Type"`
	Md5Sum           string         `json:"Md5Sum"`
}

type Bridge struct {
	cfg     *config.Config
	m       *metrics.Metrics
	tracker *state.Tracker
	ips     *state.IPMap

	prevBattery sync.Map // fsNode -> float32

	// Network counters are absolute snapshots in MQTT payload; we expose
	// them as monotonic Prometheus counters by remembering the previous
	// value per labelset and emitting Add(delta).
	prevNet sync.Map // counterID -> float64
}

func New(cfg *config.Config, m *metrics.Metrics) *Bridge {
	return &Bridge{
		cfg:     cfg,
		m:       m,
		tracker: state.NewTracker(cfg.DeliveryDeadline, cfg.PendingPutsMaxSize),
		ips:     state.NewIPMap(),
	}
}

// Handle is the central MQTT dispatch.
func (b *Bridge) Handle(_ context.Context, topic string, _ bool, data []byte) {
	switch {
	case topic == "crud-events":
		b.onCrudEvent(data)
	case strings.HasSuffix(topic, "/resources"):
		b.onResources(data)
	case strings.HasPrefix(topic, "total-network-stats/") && !strings.HasSuffix(topic, "_"):
		b.onNetworkStats(topic, data)
	case strings.HasPrefix(topic, "online-states/") && !strings.HasSuffix(topic, "_"):
		b.onOnlineState(data)
	}
}

func (b *Bridge) onCrudEvent(data []byte) {
	var e crudEvent
	if err := json.Unmarshal(data, &e); err != nil {
		slog.Warn("metrics-bridge: cannot decode crud-events", "error", err)
		return
	}
	when := e.When
	if when.IsZero() {
		when = time.Now()
	}
	switch e.Type {
	case "PUT":
		b.m.FileProducedTotal.WithLabelValues(e.FsNodeName).Inc()
		b.m.FileProducedBytesTotal.WithLabelValues(e.FsNodeName).Add(float64(e.ContentSizeBytes))
		b.tracker.RecordPut(e.Md5Sum, e.FsNodeName, e.ContentSizeBytes, when)
	case "RECEIVED":
		put := b.tracker.MatchReceive(e.Md5Sum)
		source := ""
		if put != nil {
			source = put.Source
		}
		b.m.FileReceivedTotal.WithLabelValues(e.FsNodeName, source).Inc()
		b.m.FileReceivedBytesTotal.WithLabelValues(e.FsNodeName, source).Add(float64(e.ContentSizeBytes))
		if put != nil {
			isTarget := "false"
			if b.cfg.TargetGSFor(put.Source) == e.FsNodeName {
				isTarget = "true"
			}
			b.m.FileDeliverySeconds.WithLabelValues(put.Source, e.FsNodeName, isTarget).Observe(when.Sub(put.When).Seconds())
		}
	case "DELETE":
		b.m.FileDeletedTotal.WithLabelValues(e.FsNodeName).Inc()
	}
}

func (b *Bridge) onResources(data []byte) {
	r := &proto.FsNodeResources{}
	if err := json.Unmarshal(data, r); err != nil {
		slog.Warn("metrics-bridge: cannot decode resources", "error", err)
		return
	}
	nodeType := b.ips.NodeType(r.FsNodeName)
	if r.Power != nil {
		b.m.BatteryWh.WithLabelValues(r.FsNodeName, nodeType).Set(float64(r.Power.BatteryWh))
		b.m.BatteryCapacityWh.WithLabelValues(r.FsNodeName, nodeType).Set(float64(r.Power.BatteryCapacityWh))
		b.updateConsumed(r.FsNodeName, nodeType, r.Power.BatteryWh)
		setBool(b.m.InShadow, []string{r.FsNodeName, nodeType}, r.Power.InShadow)
		setBool(b.m.LowPower, []string{r.FsNodeName, nodeType}, r.Power.Mode == proto.PowerState_LOW_POWER)
	}
	for _, v := range r.Volumes {
		b.m.VolumeUsedBytes.WithLabelValues(r.FsNodeName, nodeType, v.Name).Set(float64(v.UsedBytes))
		b.m.VolumeCapacityBytes.WithLabelValues(r.FsNodeName, nodeType, v.Name).Set(float64(v.CapacityBytes))
	}
	for _, c := range r.EngineContainers {
		b.m.ContainerCPUMilli.WithLabelValues(r.FsNodeName, nodeType, c.ContainerName).Set(float64(c.CpuMillicores))
		b.m.ContainerCPUMilliLim.WithLabelValues(r.FsNodeName, nodeType, c.ContainerName).Set(float64(c.CpuMillicoresLimit))
		b.m.ContainerMemoryBytes.WithLabelValues(r.FsNodeName, nodeType, c.ContainerName).Set(float64(c.MemoryBytes))
		b.m.ContainerMemoryLimit.WithLabelValues(r.FsNodeName, nodeType, c.ContainerName).Set(float64(c.MemoryBytesLimit))
	}
}

func (b *Bridge) updateConsumed(fsNode, nodeType string, curWh float32) {
	if v, ok := b.prevBattery.Load(fsNode); ok {
		prev := v.(float32)
		if d := prev - curWh; d > 0 {
			b.m.BatteryConsumedWhTot.WithLabelValues(fsNode, nodeType).Add(float64(d))
		}
	}
	b.prevBattery.Store(fsNode, curWh)
}

func (b *Bridge) onNetworkStats(topic string, data []byte) {
	parts := strings.Split(topic, "/")
	fsNode := parts[len(parts)-1]
	var stats []*proto.TrafficStats
	if err := json.Unmarshal(data, &stats); err != nil {
		slog.Warn("metrics-bridge: cannot decode network stats", "error", err)
		return
	}
	for _, s := range stats {
		peerIP := state.TrimPort(s.Ip)
		peerNode := b.ips.Lookup(peerIP).FsNode
		labels := []string{fsNode, peerIP, peerNode}
		b.addAbsolute("tx_bytes", b.m.NetworkTxBytesTotal, labels, float64(s.TotalBytesSent))
		b.addAbsolute("rx_bytes", b.m.NetworkRxBytesTotal, labels, float64(s.TotalBytesReceived))
		b.addAbsolute("tx_packets", b.m.NetworkTxPacketsTotal, labels, float64(s.TotalPacketsSent))
		b.addAbsolute("rx_packets", b.m.NetworkRxPacketsTotal, labels, float64(s.TotalPacketsReceived))
	}
}

func (b *Bridge) onOnlineState(data []byte) {
	s := &proto.FsNodeOnlineState{}
	if err := json.Unmarshal(data, s); err != nil {
		slog.Warn("metrics-bridge: cannot decode online state", "error", err)
		return
	}
	if s.FsNodeId == nil {
		return
	}
	b.ips.Set(s.Ip, s.FsNodeId.Name, s.NodeType)
}

// Sweep evicts expired pending PUTs and bumps yass_file_lost_total.
// Run from main on a ticker.
func (b *Bridge) Sweep(now time.Time) {
	lost := b.tracker.EvictExpired(now)
	for source, n := range lost {
		target := b.cfg.TargetGSFor(source)
		b.m.FileLostTotal.WithLabelValues(source, target).Add(float64(n))
	}
}

func setBool(g *prometheus.GaugeVec, lvs []string, v bool) {
	val := 0.0
	if v {
		val = 1
	}
	g.WithLabelValues(lvs...).Set(val)
}

// addAbsolute converts an absolute kernel-style snapshot counter into a
// monotonic Prometheus counter. We remember the last value per labelset
// and Add(curr-prev); on counter reset (curr < prev) the new value is
// treated as the fresh baseline.
func (b *Bridge) addAbsolute(metricName string, c *prometheus.CounterVec, lvs []string, curr float64) {
	key := metricName + "|" + strings.Join(lvs, "|")
	if v, ok := b.prevNet.Load(key); ok {
		prev := v.(float64)
		if curr >= prev {
			c.WithLabelValues(lvs...).Add(curr - prev)
		}
	}
	b.prevNet.Store(key, curr)
}
