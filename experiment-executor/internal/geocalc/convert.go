package geocalc

import (
	"time"

	geocalcproto "github.com/duobitx/yass-internal-components/go-common/proto/go"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
)

func Convert(input *geocalcproto.GeoCommon) (*GlobalGeoCalcUpdate, error) {
	if input == nil {
		return nil, errors.New("nil geocalc input")
	}

	tNow := time.Time{}
	if input.GetTime() != nil {
		tNow = input.GetTime().AsTime().UTC()
	}

	fsCount := len(input.GetItems())
	if expected := int(input.GetNsat() + input.GetNbs()); expected > 0 && expected < fsCount {
		fsCount = expected
	}
	fsNodesList := make([]*FsNodeInfo, fsCount)
	distances := make(map[int]map[int]float32)
	jeh := goutils.JoinErrorHelper{}
	for i := 0; i < fsCount; i++ {
		item := input.GetItems()[i]
		itemName := item.GetName()
		fsn := FsNodeInfo{
			Name: itemName,
			X:    float32(item.GetX()),
			Y:    float32(item.GetY()),
			Z:    float32(item.GetZ()),
			Lat:  float32(item.GetLat()),
			Lng:  float32(item.GetLon()),
			Alt:  float32(item.GetAlt()),
		}
		fsNodesList[i] = &fsn
	}

	for distIndex, d := range input.GetDistances() {
		if !d.GetLos() || d.GetDistance() <= 0 {
			continue
		}
		a := int(d.GetItemIdA()) - 1
		b := int(d.GetItemIdB()) - 1
		if a < 0 || a >= fsCount || b < 0 || b >= fsCount {
			jeh.Append(errors.Errorf("invalid distance refs a=%d b=%d (where: index:%d, fsCount:%d)", a, b, distIndex, fsCount))
			continue
		}
		appendDistance(&distances, a, b, d.GetDistance())
	}
	err := jeh.AsError()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse gocalc input")
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
					NameTo:   to.Name,
				}
				dInfos[index] = di
				index++
			}
			fsNode.ReachableFsNodes = dInfos
		}
	}

	up := &GlobalGeoCalcUpdate{
		CurrentTime: tNow,
		FsNodeInfos: fsNodesList,
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
