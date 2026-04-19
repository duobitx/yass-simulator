package geocalc

import (
	"time"
)

type GlobalGeoCalcUpdate struct {
	CurrentTime time.Time
	FsNodeInfos []*FsNodeInfo
}

type DistanceInfo struct {
	Distance float32
	NameTo   string
}

type FsNodeInfo struct {
	Name             string
	X                float32
	Y                float32
	Z                float32
	Lat              float32
	Lng              float32
	Alt              float32
	InShadow         bool
	ReachableFsNodes []DistanceInfo
}
