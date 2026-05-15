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
