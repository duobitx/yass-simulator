package geocalc

import (
	"strings"
	"time"

	"github.com/pkg/errors"
)

func Convert(input *common) (*GeoCalcUpdate, error) {
	timeStr := strings.TrimSpace(convBytesToString(input.UtcDttm[:]))
	tNow, err := time.ParseInLocation(time.DateTime, timeStr, time.UTC)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse time '%s'", timeStr)
	}
	fsCount := int(input.Nsat + input.Nbs)
	fsNodes := make(map[string]FsNodeInfo)
	for i := 0; i < fsCount; i++ {
		sat := input.Sats[i]
		satName := convBytesToString(sat.Name[:])
		fsn := FsNodeInfo{
			Name: satName,
			X:    float32(sat.X),
			Y:    float32(sat.Y),
			Z:    float32(sat.Z),
			Lat:  float32(sat.Lat),
			Lng:  float32(sat.Lng),
			Alt:  float32(sat.Alt),
		}
		fsNodes[satName] = fsn
	}
	up := &GeoCalcUpdate{
		BusyCount:          int(input.Busy),
		SatCount:           int(input.Nsat),
		GroundStationCount: int(input.Nsat),
		CurrentTime:        tNow,
		FsNodeInfos:        fsNodes,
	}
	return up, nil
}

func convBytesToString(buff []byte) string {
	j := len(buff) - 1
	for i := 0; i < len(buff); i++ {
		if buff[i] == 0 {
			j = i
			break
		}
	}
	buff = buff[:j]
	return string(buff)
}
