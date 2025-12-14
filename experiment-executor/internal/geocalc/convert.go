package geocalc

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type distanceKey struct {
	indexLo int
	indexHi int
}

func Convert(input *common) (*GeoCalcUpdate, error) {
	timeStr := convBytesToString(input.UtcDttm[:])
	tNow, err := time.ParseInLocation(time.DateTime, timeStr, time.UTC)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse time '%s'", timeStr)
	}
	fsCount := int(input.Nsat + input.Nbs)
	fsNodesList := make([]*FsNodeInfo, fsCount)
	distances := make(map[int]map[int]float32)
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
		fsNodesList[i] = &fsn
		for _, d := range sat.SatRef {
			if d.Dist <= 0.001 {
				continue
			}
			appendDistance(&distances, i, int(d.Sid), d.Dist)
			appendDistance(&distances, int(d.Sid), i, d.Dist)
		}
	}

	// fill DistanceInfo for each fsNode
	for i, fsNode := range fsNodesList {
		fsDistances, ok := distances[i]
		if !ok {
			fsNode.ReachableFsNodes = []DistanceInfo{}
		} else {
			dInfos := make([]DistanceInfo, len(fsDistances))
			index := 0
			for kIndex, vDist := range fsDistances {
				to := fsNodesList[kIndex]
				di := DistanceInfo{
					Distance: vDist,
					To:       to.Name,
				}
				dInfos[index] = di
				index++
			}
			fsNode.ReachableFsNodes = dInfos
		}
	}

	up := &GeoCalcUpdate{
		SatCount:           int(input.Nsat),
		GroundStationCount: int(input.Nbs),
		CurrentTime:        tNow,
		FsNodeInfos:        fsNodesList,
	}
	return up, nil
}

func appendDistance(distances *map[int]map[int]float32, satIndexA int, satIndexB int, dist float32) {
	if satIndexA == satIndexB {
		return
	}
	dists := *distances
	m, ok := dists[satIndexA]
	if !ok {
		m = make(map[int]float32)
		dists[satIndexA] = m
	}
	m[satIndexB] = dist
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
