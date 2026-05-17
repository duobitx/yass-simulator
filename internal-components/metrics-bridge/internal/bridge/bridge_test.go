package bridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/config"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/lokipush"
	"github.com/duobitx/yass-simulator/internal-components/metrics-bridge/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func newTestBridge(t *testing.T) (*Bridge, *metrics.Metrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := metrics.New(reg)
	cfg := &config.Config{
		ExperimentName:     "test",
		Engine:             "tus",
		RunID:              "run-1",
		DeliveryDeadline:   time.Hour,
		PendingPutsMaxSize: 100,
		TargetGSByFsNode:   map[string]string{"sat-1": "gs-a"},
	}
	return New(cfg, m, lokipush.New("", "")), m, reg
}

func counterValue(t *testing.T, c prometheus.Collector) float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 32)
	c.Collect(ch)
	close(ch)
	var sum float64
	for metric := range ch {
		var pb dto.Metric
		if err := metric.Write(&pb); err != nil {
			t.Fatal(err)
		}
		if pb.Counter != nil {
			sum += pb.Counter.GetValue()
		}
	}
	return sum
}

func histogramObservations(t *testing.T, c prometheus.Collector) (count uint64, sum float64) {
	t.Helper()
	ch := make(chan prometheus.Metric, 32)
	c.Collect(ch)
	close(ch)
	for metric := range ch {
		var pb dto.Metric
		if err := metric.Write(&pb); err != nil {
			t.Fatal(err)
		}
		if pb.Histogram != nil {
			count += pb.Histogram.GetSampleCount()
			sum += pb.Histogram.GetSampleSum()
		}
	}
	return count, sum
}

func TestCrudEventDeliveryToTargetGS(t *testing.T) {
	br, m, _ := newTestBridge(t)
	t0 := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)

	put, _ := json.Marshal(map[string]any{
		"FsNodeName":       "sat-1",
		"ContentSizeBytes": 1234,
		"When":             t0,
		"Type":             "PUT",
		"Md5Sum":           "abc",
	})
	br.Handle(context.Background(), "crud-events", false, put)

	recv, _ := json.Marshal(map[string]any{
		"FsNodeName":       "gs-a",
		"ContentSizeBytes": 1234,
		"When":             t0.Add(45 * time.Second),
		"Type":             "RECEIVED",
		"Md5Sum":           "abc",
	})
	br.Handle(context.Background(), "crud-events", false, recv)

	if v := counterValue(t, m.FileProducedTotal); v != 1 {
		t.Errorf("file_produced_total=%v, want 1", v)
	}
	if v := counterValue(t, m.FileReceivedTotal); v != 1 {
		t.Errorf("file_received_total=%v, want 1", v)
	}
	count, sum := histogramObservations(t, m.FileDeliverySeconds)
	if count != 1 {
		t.Fatalf("delivery histogram count=%d, want 1", count)
	}
	if sum != 45 {
		t.Errorf("delivery histogram sum=%v, want 45", sum)
	}
}

func TestSweepEmitsLost(t *testing.T) {
	br, m, _ := newTestBridge(t)
	old := time.Now().Add(-2 * time.Hour)
	data, _ := json.Marshal(map[string]any{
		"FsNodeName": "sat-1", "When": old, "Type": "PUT", "Md5Sum": "stale",
	})
	br.Handle(context.Background(), "crud-events", false, data)

	br.Sweep(time.Now())
	if v := counterValue(t, m.FileLostTotal); v != 1 {
		t.Errorf("file_lost_total=%v, want 1", v)
	}
}

func TestResourcesAndConsumed(t *testing.T) {
	br, m, _ := newTestBridge(t)
	r1, _ := json.Marshal(&proto.FsNodeResources{
		FsNodeName: "sat-1",
		Power:      &proto.PowerState{Mode: proto.PowerState_NORMAL, BatteryWh: 100, BatteryCapacityWh: 200, InShadow: false},
	})
	r2, _ := json.Marshal(&proto.FsNodeResources{
		FsNodeName: "sat-1",
		Power:      &proto.PowerState{Mode: proto.PowerState_LOW_POWER, BatteryWh: 92, BatteryCapacityWh: 200, InShadow: true},
	})
	br.Handle(context.Background(), "sat-1/resources", false, r1)
	br.Handle(context.Background(), "sat-1/resources", false, r2)

	if v := counterValue(t, m.BatteryConsumedWhTot); v != 8 {
		t.Errorf("battery_consumed_wh_total=%v, want 8", v)
	}
}

func TestNetworkStatsDelta(t *testing.T) {
	br, m, _ := newTestBridge(t)
	first, _ := json.Marshal([]*proto.TrafficStats{{Ip: "10.0.0.5", TotalBytesSent: 100, TotalBytesReceived: 200}})
	second, _ := json.Marshal([]*proto.TrafficStats{{Ip: "10.0.0.5", TotalBytesSent: 350, TotalBytesReceived: 600}})
	br.Handle(context.Background(), "total-network-stats/sat-1", false, first)
	br.Handle(context.Background(), "total-network-stats/sat-1", false, second)

	if v := counterValue(t, m.NetworkTxBytesTotal); v != 250 {
		t.Errorf("tx_bytes_total=%v, want 250", v)
	}
	if v := counterValue(t, m.NetworkRxBytesTotal); v != 400 {
		t.Errorf("rx_bytes_total=%v, want 400", v)
	}
}

func TestOnlineStatePopulatesIPMap(t *testing.T) {
	br, m, _ := newTestBridge(t)
	state, _ := json.Marshal(&proto.FsNodeOnlineState{
		FsNodeId: &proto.FsNodeId{Name: "gs-a", Experiment: "test"},
		Ip:       "10.0.0.7",
		Online:   true,
		NodeType: "groundStation",
	})
	br.Handle(context.Background(), "online-states/gs-a", false, state)

	traffic, _ := json.Marshal([]*proto.TrafficStats{{Ip: "10.0.0.7", TotalBytesSent: 50}})
	br.Handle(context.Background(), "total-network-stats/sat-1", false, traffic)
	br.Handle(context.Background(), "total-network-stats/sat-1", false, traffic)

	// Just ensure that the peer_node label was populated by checking
	// that the metric exists with non-empty peer_node. Easiest path:
	// gather and look at any series under NetworkTxBytesTotal.
	ch := make(chan prometheus.Metric, 4)
	m.NetworkTxBytesTotal.Collect(ch)
	close(ch)
	found := false
	for metric := range ch {
		var pb dto.Metric
		if err := metric.Write(&pb); err != nil {
			t.Fatal(err)
		}
		for _, l := range pb.Label {
			if l.GetName() == "peer_node" && l.GetValue() == "gs-a" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected a network-tx series labelled peer_node=gs-a")
	}
}
