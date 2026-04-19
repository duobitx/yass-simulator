package conv

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/duobitx/yass-internal-components/events-webapp/pkg/api"
	proto "github.com/duobitx/yass-internal-components/go-common/proto/go"
)

func timestamp(millis int64) time.Time {
	return time.Unix(0, millis*int64(time.Millisecond))
}

type CFunc func(topic string, data []byte) (any, error)

func FsNodeUpdateConv(_ string, data []byte) (any, error) {
	in := &proto.FsNodeUpdate{}
	err := json.Unmarshal(data, in)
	if err != nil {
		return nil, err
	}
	out := api.PositionEvent{
		BaseEvent: api.BaseEvent{
			Source:    in.Name,
			Timestamp: timestamp(in.UpdatedUnixMillis),
			EventType: "PositionEvent",
		},
		X:   in.X,
		Y:   in.Y,
		Z:   in.Z,
		Lat: in.Lat,
		Lng: in.Lng,
		Alt: in.Alt,
	}
	return out, nil
}

func FsNodeNetworkUsageConv(topic string, data []byte) (any, error) {
	in := make([]*proto.TrafficStats, 0)
	err := json.Unmarshal(data, &in)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(topic, "/")
	name := parts[len(parts)-1]
	out := api.NetworkUsageEvent{
		BaseEvent: api.BaseEvent{
			Source:    name,
			Timestamp: time.Now(),
			EventType: "NetworkUsageEvent",
		},
	}
	for i := 0; i < len(in); i++ {
		stat := in[i]
		out.TotalPacketsSent += stat.TotalPacketsSent
		out.TotalPacketsReceived += stat.TotalPacketsReceived
		out.TotalBytesReceived += stat.TotalBytesReceived
		out.TotalBytesSent += stat.TotalBytesSent
	}
	return out, nil
}
