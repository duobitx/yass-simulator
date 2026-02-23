package api

import "time"

type BaseEvent struct {
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"eventType"`
}

type PositionEvent struct {
	BaseEvent
	X   float32
	Y   float32
	Z   float32
	Lat float32
	Lng float32
	Alt float32
}
