package geocalc

import "time"

type GeoCalcUpdate struct {
	BusyCount          int
	SatCount           int
	GroundStationCount int
	CurrentTime        time.Time
	FsNodeInfos        map[string]FsNodeInfo
}

type FsNodeInfo struct {
	Name string
	X    float32
	Y    float32
	Z    float32
	Lat  float32
	Lng  float32
	Alt  float32
}
