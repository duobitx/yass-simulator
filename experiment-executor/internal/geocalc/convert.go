package geocalc

import (
	"math"
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
	jeh := goutils.JoinErrorHelper{}
	for i := 0; i < fsCount; i++ {
		item := input.GetItems()[i]
		itemName := item.GetName()
		fsn := FsNodeInfo{
			Name:             itemName,
			X:                float32(item.GetX()),
			Y:                float32(item.GetY()),
			Z:                float32(item.GetZ()),
			Lat:              float32(item.GetLat()),
			Lng:              float32(item.GetLon()),
			Alt:              float32(item.GetAlt()),
			ReachableFsNodes: []DistanceInfo{},
		}
		fsNodesList[i] = &fsn
	}

	for distIndex, d := range input.GetDistances() {
		if !d.GetLos() || d.GetDistance() <= 0 {
			continue
		}
		a := int(d.GetItemIdA()) - 1
		b := int(d.GetItemIdB()) - 1
		if a == b {
			continue
		}
		if a < 0 || a >= fsCount || b < 0 || b >= fsCount {
			jeh.Append(errors.Errorf("invalid distance refs a=%d b=%d (where: index:%d, fsCount:%d)", a, b, distIndex, fsCount))
			continue
		}
		appendDistance(&fsNodesList, a, b, d.GetDistance())
		appendDistance(&fsNodesList, b, a, d.GetDistance())
	}
	err := jeh.AsError()
	if err != nil {
		return nil, errors.Wrapf(err, "cannot parse gocalc input")
	}

	up := &GlobalGeoCalcUpdate{
		CurrentTime: tNow,
		FsNodeInfos: fsNodesList,
	}
	return up, nil
}

func appendDistance(to *[]*FsNodeInfo, refIndex int, aIndex int, dist float32) {
	distances := (*to)[refIndex].ReachableFsNodes
	toName := (*to)[aIndex].Name
	for _, d := range distances {
		if d.NameTo == toName {
			return
		}
	}
	distances = append(distances, DistanceInfo{
		Distance: float32(math.Abs(float64(dist))),
		NameTo:   toName,
	})
	(*to)[refIndex].ReachableFsNodes = distances
}
