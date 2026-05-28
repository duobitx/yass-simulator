package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	FileProducedTotal      *prometheus.CounterVec
	FileProducedBytesTotal *prometheus.CounterVec
	FileReceivedTotal      *prometheus.CounterVec
	FileReceivedBytesTotal *prometheus.CounterVec
	FileDeletedTotal       *prometheus.CounterVec
	FileLostTotal          *prometheus.CounterVec
	FileDeliverySeconds    *prometheus.HistogramVec

	BatteryWh             *prometheus.GaugeVec
	BatteryCapacityWh     *prometheus.GaugeVec
	BatteryConsumedWhTot  *prometheus.CounterVec
	InShadow              *prometheus.GaugeVec
	LowPower              *prometheus.GaugeVec
	VolumeUsedBytes       *prometheus.GaugeVec
	VolumeCapacityBytes   *prometheus.GaugeVec
	ContainerCPUMilli     *prometheus.GaugeVec
	ContainerCPUMilliLim  *prometheus.GaugeVec
	ContainerMemoryBytes  *prometheus.GaugeVec
	ContainerMemoryLimit  *prometheus.GaugeVec

	NetworkTxBytesTotal   *prometheus.CounterVec
	NetworkRxBytesTotal   *prometheus.CounterVec
	NetworkTxPacketsTotal *prometheus.CounterVec
	NetworkRxPacketsTotal *prometheus.CounterVec

	HardwareEventActive       *prometheus.GaugeVec
	HardwareEventDroppedTotal *prometheus.CounterVec

	LosActive *prometheus.GaugeVec

	// EDFS replica tracking — Tier 1 in obs-v2-spec §G4.
	// Engines publish snapshots on edfs-cids/<fsNode>; the bridge derives
	// per-CID gauges. Per-block tracking is Tier 2 (Loki only).
	EdfsBlocksPresent       *prometheus.GaugeVec
	EdfsBlocksTotal         *prometheus.GaugeVec
	EdfsReplicaCompleteness *prometheus.GaugeVec
}

func deliveryBuckets() []float64 {
	return []float64{1, 5, 15, 30, 60, 120, 300, 600, 1800, 3600, 7200}
}

func New(reg prometheus.Registerer) *Metrics {
	labelsFile := []string{"source_fsNode"}
	labelsRecv := []string{"fsNode", "source_fsNode"}
	labelsDel := []string{"source_fsNode", "target_fsNode", "is_target_gs"}
	labelsFsNode := []string{"fsNode", "node_type"}
	labelsVolume := []string{"fsNode", "node_type", "volume"}
	labelsContainer := []string{"fsNode", "node_type", "container"}
	labelsNet := []string{"fsNode", "peer", "peer_node"}

	m := &Metrics{
		FileProducedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "yass_file_produced_total", Help: "Files produced (PUT) by source fsNode."},
			labelsFile),
		FileProducedBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "yass_file_produced_bytes_total", Help: "Bytes produced (PUT) by source fsNode."},
			labelsFile),
		FileReceivedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "yass_file_received_total", Help: "Files received by an fsNode."},
			labelsRecv),
		FileReceivedBytesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "yass_file_received_bytes_total", Help: "Bytes received by an fsNode."},
			labelsRecv),
		FileDeletedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "yass_file_deleted_total", Help: "Files deleted on an fsNode."},
			[]string{"fsNode"}),
		FileLostTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "yass_file_lost_total", Help: "Files that were produced but never delivered within DELIVERY_DEADLINE."},
			[]string{"source_fsNode", "target_fsNode"}),
		FileDeliverySeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{Name: "yass_file_delivery_seconds", Help: "Seconds between PUT on source and RECEIVED on destination.", Buckets: deliveryBuckets()},
			labelsDel),

		BatteryWh:            prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_battery_wh"}, labelsFsNode),
		BatteryCapacityWh:    prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_battery_capacity_wh"}, labelsFsNode),
		BatteryConsumedWhTot: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "yass_battery_consumed_wh_total"}, labelsFsNode),
		InShadow:             prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_in_shadow"}, labelsFsNode),
		LowPower:             prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_low_power"}, labelsFsNode),
		VolumeUsedBytes:      prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_volume_used_bytes"}, labelsVolume),
		VolumeCapacityBytes:  prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_volume_capacity_bytes"}, labelsVolume),
		ContainerCPUMilli:    prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_container_cpu_millicores"}, labelsContainer),
		ContainerCPUMilliLim: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_container_cpu_millicores_limit"}, labelsContainer),
		ContainerMemoryBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_container_memory_bytes"}, labelsContainer),
		ContainerMemoryLimit: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "yass_container_memory_bytes_limit"}, labelsContainer),

		NetworkTxBytesTotal:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "yass_network_tx_bytes_total"}, labelsNet),
		NetworkRxBytesTotal:   prometheus.NewCounterVec(prometheus.CounterOpts{Name: "yass_network_rx_bytes_total"}, labelsNet),
		NetworkTxPacketsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "yass_network_tx_packets_total"}, labelsNet),
		NetworkRxPacketsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "yass_network_rx_packets_total"}, labelsNet),

		HardwareEventActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "yass_hardware_event_active", Help: "1 while a hardware-event fault is active on the fsNode (per type)."},
			[]string{"fsNode", "type"}),
		HardwareEventDroppedTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "yass_hardware_event_dropped_total", Help: "Hardware-event occurrences dropped (e.g. overlap with already-active event of the same type)."},
			[]string{"fsNode", "type", "reason"}),

		LosActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "yass_los_active", Help: "1 while there is line-of-sight from `fsNode` to `peer` per the executor's geo-calc tick."},
			[]string{"fsNode", "peer"}),

		EdfsBlocksPresent: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "yass_edfs_blocks_present", Help: "Number of locally-pinned blocks of root CID on fsNode."},
			[]string{"cid", "fsNode"}),
		EdfsBlocksTotal: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "yass_edfs_blocks_total", Help: "Total number of blocks the root CID is made of (advertised by the source's engine)."},
			[]string{"cid"}),
		EdfsReplicaCompleteness: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "yass_edfs_replica_completeness", Help: "0..1 fraction of CID's blocks present on fsNode. 1.0 = full replica."},
			[]string{"cid", "fsNode"}),
	}
	reg.MustRegister(
		m.FileProducedTotal, m.FileProducedBytesTotal,
		m.FileReceivedTotal, m.FileReceivedBytesTotal,
		m.FileDeletedTotal, m.FileLostTotal, m.FileDeliverySeconds,
		m.BatteryWh, m.BatteryCapacityWh, m.BatteryConsumedWhTot,
		m.InShadow, m.LowPower, m.VolumeUsedBytes, m.VolumeCapacityBytes,
		m.ContainerCPUMilli, m.ContainerCPUMilliLim, m.ContainerMemoryBytes, m.ContainerMemoryLimit,
		m.NetworkTxBytesTotal, m.NetworkRxBytesTotal,
		m.NetworkTxPacketsTotal, m.NetworkRxPacketsTotal,
		m.HardwareEventActive, m.HardwareEventDroppedTotal,
		m.LosActive,
		m.EdfsBlocksPresent, m.EdfsBlocksTotal, m.EdfsReplicaCompleteness,
	)
	return m
}
