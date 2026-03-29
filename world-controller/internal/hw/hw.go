package hw

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/duobitx/yass-internal-components/go-common/proto/go"
	yassv1 "github.com/duobitx/yass-operator/api/v1"
	"k8s.io/apimachinery/pkg/util/json"
)

type NodeHwState struct {
	lock              sync.Mutex
	hw                *yassv1.HardwareSpec
	batteryLevel      float32
	energyConsumption float32
	totalByteOutPerIP map[string]uint64
	lastUpdate        time.Time
	InShadow          bool
}

func (s *NodeHwState) Update(tStats []*proto.TrafficStats) ([]byte, string, error) {
	now := time.Now()
	s.lock.Lock()
	sumBytesOutThisTurn := uint64(0)
	for _, ts := range tStats {
		if ts != nil {
			prevBytesOut, ok := s.totalByteOutPerIP[ts.Ip]
			if !ok {
				s.totalByteOutPerIP[ts.Ip] = ts.TotalBytesSent
				sumBytesOutThisTurn += ts.TotalBytesSent
			} else {
				diff := ts.TotalBytesSent - prevBytesOut
				s.totalByteOutPerIP[ts.Ip] = ts.TotalBytesSent
				if diff > 0 {
					sumBytesOutThisTurn += diff
				}
			}
		}
	}
	defer s.lock.Unlock()
	t := now.Sub(s.lastUpdate).Seconds()
	drain := float64(s.hw.EnergyConsumption.NormalPowerBaseW)*t + float64(float32(sumBytesOutThisTurn)*s.hw.EnergyConsumption.PerkByteTXWh)
	gain := 0.0
	if !s.InShadow {
		gain = float64(s.hw.BatteryChargeW) * t
	}
	change := float32(gain - drain)
	if s.batteryLevel > s.hw.BatteryCapacityWh {
		s.batteryLevel = s.hw.BatteryCapacityWh
		change = 0
	} else {
		s.batteryLevel += change / 3600.0
	}
	lowPowerMode := s.batteryLevel <= s.hw.LowPowerThresholdWh
	slog.Info("NodeHwState.Update.battery", "change", change, "newLevel", s.batteryLevel, "drain", drain, "gain", gain, "lowPowerMode", lowPowerMode)
	if lowPowerMode {
		s.energyConsumption = s.hw.EnergyConsumption.LowPowerBaseW
	} else {
		s.energyConsumption = s.hw.EnergyConsumption.NormalPowerBaseW
	}
	s.lastUpdate = now
	buff, err := json.Marshal(s)
	return buff, s.format(change), err
}

func (s *NodeHwState) format(change float32) string {
	var str string
	if s.hw.EnergyConsumption.LowPowerBaseW <= 0 {
		str = "-"
	} else {
		lev := float64(s.batteryLevel) / float64(s.hw.BatteryCapacityWh) * 100.0
		str = fmt.Sprintf("%d%%,%+.1fW", int(lev), change)
	}
	if !s.InShadow {
		str += " ☀"
	}
	return str
}

func NewNodeHwState(spec *yassv1.HardwareSpec) *NodeHwState {
	return &NodeHwState{
		lock:              sync.Mutex{},
		hw:                spec,
		batteryLevel:      spec.BatteryCapacityWh,
		energyConsumption: spec.EnergyConsumption.NormalPowerBaseW,
		totalByteOutPerIP: make(map[string]uint64),
		lastUpdate:        time.Time{},
		InShadow:          false,
	}
}
