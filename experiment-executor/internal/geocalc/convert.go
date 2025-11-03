package geocalc

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
)

func Convert(input *common) (*GeoCalcUpdate, error) {
	timeStr := convBytesToString(input.UtcDttm[:])
	tNow, err := time.ParseInLocation(time.DateTime, timeStr, time.UTC)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse time '%s'", timeStr)
	}
	fsCount := int(input.Nsat + input.Nbs)
	fsNodesList := make([]*FsNodeInfo, fsCount)
	fsNodesByNrRef := make(map[int]*FsNodeInfo)
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
		fsNodesByNrRef[i] = &fsn
		fsNodesList[i] = &fsn
	}
	for i := 0; i < fsCount; i++ {
		currSat := input.Sats[i]
		distances := make([]DistanceInfo, currSat.NRef)
		for j := 0; j < int(currSat.NRef); j++ {
			visibilityRecord := currSat.SatRef[j]
			toFs, ok := fsNodesByNrRef[int(visibilityRecord.Sid)]
			if !ok {
				return nil, fmt.Errorf("cannot find fsNode for sid number = %d", visibilityRecord.Sid)
			}
			distances[j] = DistanceInfo{
				Distance: visibilityRecord.Dist,
				To:       toFs.Name,
			}
		}
		fsNodesList[i].ReachableFsNodes = distances
	}

	up := &GeoCalcUpdate{
		SatCount:           int(input.Nsat),
		GroundStationCount: int(input.Nbs),
		CurrentTime:        tNow,
		FsNodeInfos:        fsNodesList,
	}
	return up, nil
}

func dump(input *common) string {
	buf := bytes.Buffer{}
	buf.WriteString(fmt.Sprintf("Time: %s\n", convBytesToString(input.UtcDttm[:])))
	buf.WriteString(fmt.Sprintf("Busy: %d NSat: %d Nbs: %d\n", input.Busy, input.Nsat, input.Nbs))
	count := int(input.Nsat + input.Nbs)
	for i := 0; i < count; i++ {
		sat := input.Sats[i]
		buf.WriteString(fmt.Sprintf(" Node:%3d:%s NRef:%d X:%.2f Y:%.2f Z:%.2f Lat:%.2f Lng:%.2f Alt:%.2fkm\n", i, convBytesToString(sat.Name[:]), sat.NRef, sat.X, sat.Y, sat.Z, sat.Lat, sat.Lng, sat.Alt))
		refIds := make([]int, count)
		for j := 0; j < count; j++ {
			refIds[j] = int(sat.SatRef[j].Sid)
		}
		buf.WriteString(fmt.Sprintf("   Refs: %+v\n", refIds))
	}
	return buf.String()
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
	return strings.TrimSpace(string(buff))
}
