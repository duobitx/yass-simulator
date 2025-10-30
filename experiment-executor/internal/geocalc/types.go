package geocalc

import (
	"time"
)

type GeoCalcUpdate struct {
	SatCount           int
	GroundStationCount int
	CurrentTime        time.Time
	FsNodeInfos        []*FsNodeInfo
}

type DistanceInfo struct {
	Distance float32
	To       string
}

type FsNodeInfo struct {
	Name             string
	X                float32
	Y                float32
	Z                float32
	Lat              float32
	Lng              float32
	Alt              float32
	ReachableFsNodes []DistanceInfo
}
