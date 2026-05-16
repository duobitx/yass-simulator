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
	)
	return m
}
