package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/config"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/k8sevents"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/lokipush"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/metrics"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/state"
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
	loki    *lokipush.Pusher
	events  k8sevents.Emitter

	// peerToNode maps a libp2p peer ID (published retained on edfs-peers/<node>
	// by each edfs engine) to its fsNode name, so block-receive provenance from
	// the kubo bitswap tracer (block-recv/<node>) can name the sender.
	peerToNode sync.Map // peerID(string) -> fsNode(string)

	prevBattery sync.Map // fsNode -> float32

	// Network counters are absolute snapshots in MQTT payload; we expose
	// them as monotonic Prometheus counters by remembering the previous
	// value per labelset and emitting Add(delta).
	prevNet sync.Map // counterID -> float64

	// Previous shadow/low-power state per fsNode, used to synthesise
	// power-transition events into Loki.
	prevShadow   sync.Map // fsNode -> bool
	prevLowPower sync.Map // fsNode -> bool
	prevOnline   sync.Map // fsNode -> bool

	// prevLosPeers tracks the previous peer set per fsNode so we can
	// zero out LosActive when a peer drops out of visibility.
	prevLosPeers sync.Map // fsNode -> map[peer]struct{}

	// Experiment-clock tracking. We sample updates/_time_ from the
	// experiment-executor and convert every event's wall-clock timestamp into
	// simulated experiment time so Loki, Grafana and the .ods export all show
	// the in-scenario clock.
	timeMu            sync.RWMutex
	lastExpTime       time.Time
	lastExpTimeAtWall time.Time

	// exportOnce ensures the post-experiment export runs at most once even
	// if the executor publishes the "ended" lifecycle event more than once.
	exportOnce sync.Once
}

// exportTimeout bounds the events-exporter subprocess so a hung export does
// not run forever or outlive shutdown.
const exportTimeout = 5 * time.Minute

// groundStationNodeType mirrors yassv1.FsNodeTypeGroundStation, the node-type
// string the world-controller stamps onto online-state messages.
const groundStationNodeType = "groundStation"

func New(cfg *config.Config, m *metrics.Metrics, loki *lokipush.Pusher, events k8sevents.Emitter) *Bridge {
	if events == nil {
		events = k8sevents.Noop()
	}
	return &Bridge{
		cfg:     cfg,
		m:       m,
		tracker: state.NewTracker(cfg.DeliveryDeadline, cfg.PendingPutsMaxSize),
		ips:     state.NewIPMap(),
		loki:    loki,
		events:  events,
	}
}

// baseLabels are the run-identifying labels attached to every Loki entry.
// Kept low-cardinality on purpose — anything per-event goes in the body.
func (b *Bridge) baseLabels(extra map[string]string) map[string]string {
	out := map[string]string{
		"experiment": b.cfg.ExperimentName,
		"engine":     b.cfg.Engine,
		"namespace":  b.cfg.Namespace,
	}
	for k, v := range extra {
		if v == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func (b *Bridge) pushEvent(kind, fsNode, eventType string, wallTs time.Time, payload map[string]any) {
	if wallTs.IsZero() {
		wallTs = time.Now()
	}
	expTs := b.experimentTime(wallTs)
	b.dispatch(kind, fsNode, eventType, expTs, wallTs, payload)
}

// dispatch is the single fan-out point for an event: it stamps the
// experiment + wall clocks onto the payload, then sends it to Loki and to
// the Kubernetes EventRecorder on the Experiment CR. Either sink may be a
// no-op (Loki URL unset, or in-cluster config missing) without affecting
// the other.
func (b *Bridge) dispatch(kind, fsNode, eventType string, expTs, wallTs time.Time, payload map[string]any) {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, ok := payload["fsNode"]; !ok && fsNode != "" {
		payload["fsNode"] = fsNode
	}
	payload["experimentTime"] = expTs.UTC().Format(time.RFC3339Nano)
	payload["wallTime"] = wallTs.UTC().Format(time.RFC3339Nano)

	b.events.Emit(kind, eventType, payload)

	// Loki's storage assumes timestamps live near wall-clock — old samples
	// (simulation start far in the past) are silently dropped even with
	// reject_old_samples=false. So we index by wallTs and surface the
	// experiment clock in the body; Grafana / .ods read experimentTime
	// from the JSON to display the in-scenario clock.
	if b.loki == nil || !b.loki.Enabled() {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("metrics-bridge: marshal loki body", "kind", kind, "error", err)
		return
	}
	b.loki.Push(b.baseLabels(map[string]string{
		"kind":   kind,
		"fsNode": fsNode,
		"type":   eventType,
		"run_id": b.cfg.RunID,
	}), wallTs, string(body))
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
	case topic == "experiment-lifecycle":
		b.onLifecycle(data)
	case strings.HasPrefix(topic, "hardware-events/") && !strings.HasSuffix(topic, "_"):
		b.onHardwareEvent(topic, data)
	case strings.HasPrefix(topic, "los/") && !strings.HasSuffix(topic, "_"):
		b.onLos(topic, data)
	case strings.HasPrefix(topic, "edfs-cids/") && !strings.HasSuffix(topic, "_"):
		b.onEdfsCids(topic, data)
	case strings.HasPrefix(topic, "edfs-peers/") && !strings.HasSuffix(topic, "_"):
		b.onEdfsPeers(data)
	case strings.HasPrefix(topic, "block-recv/") && !strings.HasSuffix(topic, "_"):
		b.onBlockRecv(topic, data)
	case topic == "updates/_time_":
		b.onTimeUpdate(data)
	}
}

// onTimeUpdate records the latest experiment-clock reading from the
// experiment-executor. The clock is interpolated forward by wall-clock delta
// inside experimentTime() — so a 5-second-tick reference is plenty accurate
// for per-event timestamps.
func (b *Bridge) onTimeUpdate(data []byte) {
	upd := &proto.TimeUpdate{}
	if err := json.Unmarshal(data, upd); err != nil {
		slog.Warn("metrics-bridge: cannot decode time update", "error", err)
		return
	}
	b.timeMu.Lock()
	b.lastExpTime = time.UnixMilli(upd.Now)
	b.lastExpTimeAtWall = time.Now()
	b.timeMu.Unlock()
}

// experimentTime converts a wall-clock instant into experiment time, using
// the most recent updates/_time_ tick as the reference point. If no tick has
// been seen yet (events before the experiment starts, or the executor is
// down) it returns the wall-clock instant unchanged — better an entry with
// a slightly wrong timestamp than a dropped event.
func (b *Bridge) experimentTime(wall time.Time) time.Time {
	b.timeMu.RLock()
	defer b.timeMu.RUnlock()
	if b.lastExpTimeAtWall.IsZero() {
		return wall
	}
	return b.lastExpTime.Add(wall.Sub(b.lastExpTimeAtWall))
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
	source := ""
	deliverySeconds := -1.0
	switch e.Type {
	case "PUT":
		b.m.FileProducedTotal.WithLabelValues(e.FsNodeName).Inc()
		b.m.FileProducedBytesTotal.WithLabelValues(e.FsNodeName).Add(float64(e.ContentSizeBytes))
		b.tracker.RecordPut(e.Md5Sum, e.Name, e.FsNodeName, e.ContentSizeBytes, when)
	case "RECEIVED":
		put, dup := b.tracker.MatchReceive(e.Md5Sum, e.Name, e.FsNodeName)
		if dup {
			break // already counted this (file, receiver); avoid double-count
		}
		if put != nil {
			source = put.Source
			deliverySeconds = when.Sub(put.When).Seconds()
			if deliverySeconds < 0 {
				// Cross-pod wall-clock skew (producer ahead of receiver) can
				// make the delta negative; a negative observation would corrupt
				// the histogram sum.
				deliverySeconds = 0
			}
		}
		b.m.FileReceivedTotal.WithLabelValues(e.FsNodeName, source).Inc()
		b.m.FileReceivedBytesTotal.WithLabelValues(e.FsNodeName, source).Add(float64(e.ContentSizeBytes))
		if put != nil {
			// "Delivered to ground" = the receiver is a ground station. If an
			// explicit per-satellite target map is configured, honour it;
			// otherwise classify by the receiver's node type (published on
			// online-state). Without this, sat->sat replica hops look like
			// ground deliveries and the is_target_gs="true" KPI is empty.
			isTarget := "false"
			if target := b.cfg.TargetGSFor(put.Source); target != "" {
				if target == e.FsNodeName {
					isTarget = "true"
				}
			} else if b.ips.NodeType(e.FsNodeName) == groundStationNodeType {
				isTarget = "true"
			}
			b.m.FileDeliverySeconds.WithLabelValues(put.Source, e.FsNodeName, isTarget).Observe(deliverySeconds)
			// Emit a synthetic Loki event that joins the PUT and the
			// RECEIVED into one row — lets yass-timeline draw a single
			// source→target arrow with delivery_seconds, and gives
			// events-exporter a "deliveries" sheet without a manual
			// PUT↔RECEIVED join. See yass-docs/observability-v2-spec.md §G3.
			b.pushEvent("file_delivered", e.FsNodeName, "DELIVERED", when, map[string]any{
				"source":          put.Source,
				"target":          e.FsNodeName,
				"md5":             e.Md5Sum,
				"name":            e.Name,
				"size":            e.ContentSizeBytes,
				"deliverySeconds": deliverySeconds,
				"isTargetGs":      isTarget,
			})
		}
	case "DELETE":
		b.m.FileDeletedTotal.WithLabelValues(e.FsNodeName).Inc()
	}

	payload := map[string]any{
		"fsNode":     e.FsNodeName,
		"type":       e.Type,
		"name":       e.Name,
		"size":       e.ContentSizeBytes,
		"md5":        e.Md5Sum,
		"attributes": e.Attributes,
	}
	if source != "" {
		payload["source"] = source
	}
	if deliverySeconds >= 0 {
		payload["deliverySeconds"] = deliverySeconds
	}
	b.pushEvent("crud", e.FsNodeName, e.Type, when, payload)
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
		lowPower := r.Power.Mode == proto.PowerState_LOW_POWER
		setBool(b.m.LowPower, []string{r.FsNodeName, nodeType}, lowPower)
		b.detectPowerTransition(r.FsNodeName, nodeType, r.Power.InShadow, lowPower, r.Power.BatteryWh)
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
		if peerNode == "" {
			// Peer's online-state (IP->name) hasn't propagated yet. Emitting
			// now would create a peer_node="" series whose accumulated value is
			// orphaned once the name resolves (a different labelset). Skip until
			// resolved; the delta baseline is set on the first resolved sample.
			continue
		}
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

	if prev, ok := b.prevOnline.Load(s.FsNodeId.Name); ok && prev.(bool) == s.Online {
		return // no-op transitions are not events
	}
	b.prevOnline.Store(s.FsNodeId.Name, s.Online)

	eventType := "offline"
	if s.Online {
		eventType = "online"
	}
	b.pushEvent("online_state", s.FsNodeId.Name, eventType, time.Now(), map[string]any{
		"fsNode":   s.FsNodeId.Name,
		"nodeType": s.NodeType,
		"ip":       s.Ip,
		"online":   s.Online,
	})

	// A node that went offline stops publishing los/<fsNode>, so its
	// los_active{fsNode,peer} series would stay stuck at their last value.
	// Zero them out explicitly. The reverse direction (peers that see this
	// node) is handled by the online gate on their next los tick.
	if !s.Online {
		if prev, ok := b.prevLosPeers.Load(s.FsNodeId.Name); ok {
			for peer := range prev.(map[string]struct{}) {
				b.m.LosActive.WithLabelValues(s.FsNodeId.Name, peer).Set(0)
			}
		}
	}
}

func (b *Bridge) detectPowerTransition(fsNode, nodeType string, inShadow, lowPower bool, batteryWh float32) {
	if prev, ok := b.prevShadow.Load(fsNode); !ok || prev.(bool) != inShadow {
		b.prevShadow.Store(fsNode, inShadow)
		if ok {
			eventType := "exit_shadow"
			if inShadow {
				eventType = "enter_shadow"
			}
			b.pushEvent("power", fsNode, eventType, time.Now(), map[string]any{
				"fsNode":    fsNode,
				"nodeType":  nodeType,
				"inShadow":  inShadow,
				"batteryWh": batteryWh,
			})
		}
	}
	if prev, ok := b.prevLowPower.Load(fsNode); !ok || prev.(bool) != lowPower {
		b.prevLowPower.Store(fsNode, lowPower)
		if ok {
			eventType := "exit_low_power"
			if lowPower {
				eventType = "enter_low_power"
			}
			b.pushEvent("power", fsNode, eventType, time.Now(), map[string]any{
				"fsNode":    fsNode,
				"nodeType":  nodeType,
				"lowPower":  lowPower,
				"batteryWh": batteryWh,
			})
		}
	}
}

// lifecycleEvent mirrors what the experiment-executor publishes on
// the experiment-lifecycle topic. ExpTime is the executor's own
// experiment-clock stamp and takes precedence over the wall-clock
// "when" field when present.
type lifecycleEvent struct {
	State   string         `json:"state"`
	Reason  string         `json:"reason,omitempty"`
	Comment string         `json:"comment,omitempty"`
	When    time.Time      `json:"when,omitempty"`
	ExpTime time.Time      `json:"expTime,omitempty"`
	Extra   map[string]any `json:"extra,omitempty"`
}

func (b *Bridge) onLifecycle(data []byte) {
	var e lifecycleEvent
	if err := json.Unmarshal(data, &e); err != nil {
		slog.Warn("metrics-bridge: cannot decode lifecycle", "error", err)
		return
	}
	wallTime := e.When
	if wallTime.IsZero() {
		wallTime = time.Now()
	}
	payload := map[string]any{"state": e.State}
	if e.Reason != "" {
		payload["reason"] = e.Reason
	}
	if e.Comment != "" {
		payload["comment"] = e.Comment
	}
	for k, v := range e.Extra {
		payload[k] = v
	}

	// Lifecycle events come pre-stamped with the executor's experiment
	// clock; bypass the bridge's interpolation when ExpTime is present.
	if !e.ExpTime.IsZero() {
		b.pushEventAtExp("lifecycle", "", e.State, e.ExpTime, wallTime, payload)
	} else {
		b.pushEvent("lifecycle", "", e.State, wallTime, payload)
	}

	if e.State == "ended" {
		b.exportOnce.Do(func() { go b.runExportAfterGrace() })
	}
}

// pushEventAtExp is a variant of pushEvent for callers that already know
// the experiment-clock timestamp (lifecycle events from the executor).
// It skips the bridge's wall→exp interpolation and writes the provided
// expTs into the body; Loki itself is still indexed by wall time (see
// pushEvent for the reason).
func (b *Bridge) pushEventAtExp(kind, fsNode, eventType string, expTs, wallTs time.Time, payload map[string]any) {
	if wallTs.IsZero() {
		wallTs = time.Now()
	}
	b.dispatch(kind, fsNode, eventType, expTs, wallTs, payload)
}

// runExportAfterGrace waits a few seconds for the in-flight loki pushes to
// drain, then invokes the events-exporter binary (shipped in the same
// container image) to drop an .ods at the configured path.
func (b *Bridge) runExportAfterGrace() {
	if b.cfg.ExporterBin == "" {
		slog.Info("export skipped: EXPORTER_BIN not set")
		return
	}
	time.Sleep(b.cfg.ExportGrace)
	if err := b.loki.Flush(context.Background()); err != nil {
		slog.Warn("loki flush before export failed", "error", err)
	}
	args := []string{
		"--loki", b.cfg.LokiURL,
		"--experiment", b.cfg.ExperimentName,
		"--engine", b.cfg.Engine,
		"--run-id", b.cfg.RunID,
		"--since", b.cfg.ExportLookback.String(),
	}
	if b.cfg.LokiTenant != "" {
		args = append(args, "--tenant", b.cfg.LokiTenant)
	}
	if b.cfg.ExportDir != "" {
		args = append(args, "--out", strings.TrimRight(b.cfg.ExportDir, "/")+"/"+b.cfg.ExperimentName+"-"+b.cfg.RunID+".ods")
	}
	slog.Info("running events-exporter", "bin", b.cfg.ExporterBin, "args", args)
	ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.cfg.ExporterBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("events-exporter failed", "error", err, "output", string(out))
		return
	}
	slog.Info("events-exporter ok", "output", string(out))
}

// onEdfsCids handles `edfs-cids/<fsNode>` — periodic snapshot of which
// root CIDs this fsNode currently has and how complete each replica is.
// See yass-docs/observability-v2-spec.md §G4 Tier 1.
//
// Expected payload:
//
//	{"fsNode":"oneweb-0008",
//	 "snapshotAt":"...",
//	 "cids":[{"cid":"bafy...","totalBlocks":12,"presentBlocks":7}]}
func (b *Bridge) onEdfsCids(topic string, data []byte) {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return
	}
	fsNode := parts[1]
	var msg struct {
		Cids []struct {
			CID           string `json:"cid"`
			TotalBlocks   int64  `json:"totalBlocks"`
			PresentBlocks int64  `json:"presentBlocks"`
		} `json:"cids"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("metrics-bridge: cannot decode edfs-cids", "topic", topic, "error", err)
		return
	}
	for _, c := range msg.Cids {
		if c.CID == "" {
			continue
		}
		if c.TotalBlocks > 0 {
			b.m.EdfsBlocksTotal.WithLabelValues(c.CID).Set(float64(c.TotalBlocks))
		}
		b.m.EdfsBlocksPresent.WithLabelValues(c.CID, fsNode).Set(float64(c.PresentBlocks))
		completeness := 0.0
		if c.TotalBlocks > 0 {
			completeness = float64(c.PresentBlocks) / float64(c.TotalBlocks)
		}
		b.m.EdfsReplicaCompleteness.WithLabelValues(c.CID, fsNode).Set(completeness)
	}
}

// onEdfsPeers records the peerID->fsNode mapping each edfs engine publishes
// (retained) on edfs-peers/<node>. Used to name the sender in block-receive
// provenance.
func (b *Bridge) onEdfsPeers(data []byte) {
	var pi struct {
		NodeName string `json:"nodeName"`
		PeerID   string `json:"peerID"`
	}
	if err := json.Unmarshal(data, &pi); err != nil || pi.PeerID == "" || pi.NodeName == "" {
		return
	}
	b.peerToNode.Store(pi.PeerID, pi.NodeName)
}

// onBlockRecv turns the kubo bitswap tracer's per-block reception reports
// (block-recv/<toFsNode>, payload {from:peerID, t:unixMs, blocks:[{file,size}]};
// file is the tracer-resolved name, or the CID if unresolved) into one Loki
// event per block, resolving the sender peerID to its fsNode. Each event is the
// authoritative edge "from_fsNode -> to_fsNode" for one block of a file, which
// the report aggregates into a per-file propagation graph. See
// yass-docs/observability-v2-spec.md §G4.
func (b *Bridge) onBlockRecv(topic string, data []byte) {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 || parts[1] == "" {
		return
	}
	to := parts[1]
	var msg struct {
		From   string `json:"from"`
		When   int64  `json:"t"`
		Blocks []struct {
			File string `json:"file"` // file name (tracer-resolved; CID if unresolved)
			Size int64  `json:"size"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("metrics-bridge: cannot decode block-recv", "topic", topic, "error", err)
		return
	}
	from := msg.From // fall back to the raw peerID if the mapping isn't known yet
	if n, ok := b.peerToNode.Load(msg.From); ok {
		from = n.(string)
	}
	when := time.Now()
	if msg.When > 0 {
		when = time.UnixMilli(msg.When)
	}
	for _, blk := range msg.Blocks {
		if blk.File == "" {
			continue
		}
		b.pushEvent("block_recv", to, "RECV", when, map[string]any{
			"from_fsNode": from,
			"to_fsNode":   to,
			"from_peer":   msg.From,
			"file":        blk.File,
			"size":        blk.Size,
		})
	}
}

// onLos handles `los/<fsNode>` — the executor's per-tick peer roster
// derived from geo-calc visibility. We toggle yass_los_active{fsNode,
// peer} to 1 for every peer currently in the roster, and to 0 for
// peers that were visible last tick but aren't now.
// isOnline reports the last known online state of a node. Unknown (no
// online-state seen yet) defaults to true so LOS is not suppressed before the
// first online-state propagates.
func (b *Bridge) isOnline(fsNode string) bool {
	v, ok := b.prevOnline.Load(fsNode)
	if !ok {
		return true
	}
	return v.(bool)
}

func (b *Bridge) onLos(topic string, data []byte) {
	parts := strings.Split(topic, "/")
	if len(parts) < 2 {
		return
	}
	fsNode := parts[1]
	var msg struct {
		Peers []struct {
			Name string `json:"name"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(data, &msg); err != nil {
		slog.Warn("metrics-bridge: cannot decode los message", "topic", topic, "error", err)
		return
	}
	// A geometric LOS link is only "active" if both endpoints are actually
	// up. The roster comes from geo-calc visibility, which is unaware of
	// online state, so a destroyed/offline peer would otherwise keep
	// los_active=1 during large-scale failures (UC4/UC5).
	srcUp := b.isOnline(fsNode)
	now := map[string]struct{}{}
	for _, p := range msg.Peers {
		if p.Name == "" {
			continue
		}
		now[p.Name] = struct{}{}
		active := 0.0
		if srcUp && b.isOnline(p.Name) {
			active = 1
		}
		b.m.LosActive.WithLabelValues(fsNode, p.Name).Set(active)
	}
	if prev, ok := b.prevLosPeers.Load(fsNode); ok {
		for peer := range prev.(map[string]struct{}) {
			if _, still := now[peer]; !still {
				b.m.LosActive.WithLabelValues(fsNode, peer).Set(0)
			}
		}
	}
	b.prevLosPeers.Store(fsNode, now)
}

func (b *Bridge) onHardwareEvent(topic string, data []byte) {
	parts := strings.Split(topic, "/")
	fsNode := ""
	if len(parts) >= 2 {
		fsNode = parts[1]
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		payload = map[string]any{"raw": string(data)}
	}
	eventType, _ := payload["type"].(string)
	state, _ := payload["state"].(string)
	reason, _ := payload["reason"].(string)
	if eventType != "" {
		switch state {
		case "active":
			b.m.HardwareEventActive.WithLabelValues(fsNode, eventType).Set(1)
		case "cleared":
			b.m.HardwareEventActive.WithLabelValues(fsNode, eventType).Set(0)
		case "dropped_overlap":
			b.m.HardwareEventDroppedTotal.WithLabelValues(fsNode, eventType, reason).Inc()
		}
	}
	b.pushEvent("hardware", fsNode, eventType, time.Now(), payload)
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
