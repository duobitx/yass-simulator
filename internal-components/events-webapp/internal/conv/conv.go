package conv

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/events-webapp/pkg/api"
	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
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
	for _, np := range in.NetworkParams {
		out.NetworkParams = append(out.NetworkParams, api.NetworkLink{
			Subject:      np.Subject,
			IP:           np.Ip,
			Distance:     np.Distance,
			PackageDelay: np.PackageDelay,
			PackageLoss:  np.PackageLoss,
			Bandwidth:    np.Bandwidth,
		})
	}
	return out, nil
}

func FsNodeResourcesConv(_ string, data []byte) (any, error) {
	in := &proto.FsNodeResources{}
	err := json.Unmarshal(data, in)
	if err != nil {
		return nil, err
	}
	out := api.ResourceEvent{
		BaseEvent: api.BaseEvent{
			Source:    in.FsNodeName,
			Timestamp: timestamp(in.UpdatedUnixMillis),
			EventType: "ResourceEvent",
		},
	}
	if p := in.Power; p != nil {
		out.PowerMode = p.Mode.String()
		out.BatteryWh = p.BatteryWh
		out.BatteryCapacityWh = p.BatteryCapacityWh
		out.InShadow = p.InShadow
	}
	for _, v := range in.Volumes {
		out.Volumes = append(out.Volumes, api.VolumeUsage{
			Name:          v.Name,
			MountPath:     v.MountPath,
			UsedBytes:     v.UsedBytes,
			CapacityBytes: v.CapacityBytes,
			HardLimited:   v.HardLimited,
		})
	}
	for _, c := range in.EngineContainers {
		out.EngineContainers = append(out.EngineContainers, api.ContainerCompute{
			ContainerName:      c.ContainerName,
			CPUMillicores:      c.CpuMillicores,
			MemoryBytes:        c.MemoryBytes,
			CPUMillicoresLimit: c.CpuMillicoresLimit,
			MemoryBytesLimit:   c.MemoryBytesLimit,
		})
	}
	return out, nil
}

// crudNotifyEvent matches the JSON shape published by
// fs_engine_wrapper/pkg/notifier.NotifyEvent on the `crud-events` topic.
type crudNotifyEvent struct {
	Name             string    `json:"Name"`
	ContentSizeBytes int64     `json:"ContentSizeBytes"`
	FsNodeName       string    `json:"FsNodeName"`
	When             time.Time `json:"When"`
	Type             string    `json:"Type"`
	Md5Sum           string    `json:"Md5Sum"`
}

func AgentFileEventConv(_ string, data []byte) (any, error) {
	in := &crudNotifyEvent{}
	if err := json.Unmarshal(data, in); err != nil {
		return nil, err
	}
	if in.FsNodeName == "" || in.Type == "" {
		return nil, nil
	}
	ts := in.When
	if ts.IsZero() {
		ts = time.Now()
	}
	return api.AgentFileEvent{
		BaseEvent: api.BaseEvent{
			Source:    in.FsNodeName,
			Timestamp: ts,
			EventType: "AgentFileEvent",
		},
		Action:           in.Type,
		FileName:         in.Name,
		ContentSizeBytes: in.ContentSizeBytes,
		Md5:              in.Md5Sum,
	}, nil
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
		out.Peers = append(out.Peers, api.PeerUsage{
			IP:                   stat.Ip,
			PeerFsNode:           stat.PeerFsNode,
			TotalBytesSent:       stat.TotalBytesSent,
			TotalBytesReceived:   stat.TotalBytesReceived,
			TotalPacketsSent:     stat.TotalPacketsSent,
			TotalPacketsReceived: stat.TotalPacketsReceived,
		})
	}
	return out, nil
}
