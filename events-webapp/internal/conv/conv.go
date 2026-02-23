package conv

import (
	"time"

	"github.com/duobitx/yass-internal-components/events-webapp/pkg/api"
	"github.com/duobitx/yass-internal-components/go-common/com"
	"github.com/duobitx/yass-internal-components/go-common/proto"
)

func timestamp(millis int64) time.Time {
	return time.Unix(0, millis*int64(time.Millisecond))
}

type CFunc func(topic string, data []byte) (any, error)

func FsNodeUpdateConv(_ string, data []byte) (any, error) {
	in := &proto.FsNodeUpdate{}
	err := com.MsgUnmarshall(data, in)
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
