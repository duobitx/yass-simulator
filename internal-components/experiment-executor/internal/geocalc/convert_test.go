package geocalc

import (
	"testing"

	geocalcproto "github.com/duobitx/yass-internal-components/go-common/proto/go"
	"github.com/stretchr/testify/assert"
)

func TestConvert_DistanceCounts(t *testing.T) {
	input := &geocalcproto.GeoCommon{
		Nsat: 2,
		Nbs:  1,
		Items: []*geocalcproto.Item{
			{Id: 1, Name: "Node1"},
			{Id: 2, Name: "Node2"},
			{Id: 3, Name: "Node3"},
		},
		Distances: []*geocalcproto.Distance{
			{ItemIdA: 1, ItemIdB: 2, Distance: 100.0, Los: true},
			{ItemIdA: 1, ItemIdB: 3, Distance: 200.0, Los: true},
		},
	}

	result, err := Convert(input)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 3, len(result.FsNodeInfos))

	// For Node1 (index 0), it should have Node2 in ReachableFsNodes
	assert.Equal(t, 1, len(result.FsNodeInfos[0].ReachableFsNodes), "Node1 should have 1 reachable node")
	assert.Equal(t, "Node2", result.FsNodeInfos[0].ReachableFsNodes[0].NameTo)

	// For Node2 (index 1), it should have Node1 in ReachableFsNodes
	assert.Equal(t, 1, len(result.FsNodeInfos[1].ReachableFsNodes), "Node2 should have 1 reachable node")
	assert.Equal(t, "Node1", result.FsNodeInfos[1].ReachableFsNodes[0].NameTo)
}
