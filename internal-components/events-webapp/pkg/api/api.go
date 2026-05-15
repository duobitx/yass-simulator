package api

import "time"

type BaseEvent struct {
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"eventType"`
}

type NetworkLink struct {
	Subject      string  `json:"subject"`
	IP           string  `json:"ip"`
	Distance     float32 `json:"distance"`
	PackageDelay float32 `json:"packageDelay"`
	PackageLoss  float32 `json:"packageLoss"`
	Bandwidth    float32 `json:"bandwidth"`
}

type PositionEvent struct {
	BaseEvent
	X             float32
	Y             float32
	Z             float32
	Lat           float32
	Lng           float32
	Alt           float32
	NetworkParams []NetworkLink `json:"networkParams,omitempty"`
}

type NetworkUsageEvent struct {
	BaseEvent
	TotalBytesSent       uint64
	TotalBytesReceived   uint64
	TotalPacketsSent     uint64
	TotalPacketsReceived uint64
}

type VolumeUsage struct {
	Name          string `json:"name"`
	MountPath     string `json:"mountPath"`
	UsedBytes     uint64 `json:"usedBytes"`
	CapacityBytes uint64 `json:"capacityBytes"`
	HardLimited   bool   `json:"hardLimited"`
}

type ContainerCompute struct {
	ContainerName      string  `json:"containerName"`
	CPUMillicores      float32 `json:"cpuMillicores"`
	MemoryBytes        uint64  `json:"memoryBytes"`
	CPUMillicoresLimit float32 `json:"cpuMillicoresLimit,omitempty"`
	MemoryBytesLimit   uint64  `json:"memoryBytesLimit,omitempty"`
}

type ResourceEvent struct {
	BaseEvent
	PowerMode         string             `json:"powerMode"`
	BatteryWh         float32            `json:"batteryWh"`
	BatteryCapacityWh float32            `json:"batteryCapacityWh"`
	InShadow          bool               `json:"inShadow"`
	Volumes           []VolumeUsage      `json:"volumes,omitempty"`
	EngineContainers  []ContainerCompute `json:"engineContainers,omitempty"`
}
