package cmodel

import "time"

type GeoCoordinates struct {
	Lat float32
	Lng float32
}
type Rotation struct {
	Yaw   float32
	Roll  float32
	Pitch float32
}

type FsNode struct {
	ExperimentFsNode
}

type ExperimentFsNode struct {
	Name     string
	TLE      []string
	Geo      *GeoCoordinates
	Rotation Rotation
}

type ExperimentDefinition struct {
	Name        string
	StartTime   *time.Time
	MaxDuration *time.Duration
	FsNodes     []ExperimentFsNode
}
