package hw

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	proto "github.com/duobitx/yass-simulator/internal-components/go-common/proto/go"
	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"github.com/m-szalik/goutils"
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
	// First update only establishes the baseline timestamp; with a zero
	// lastUpdate the elapsed interval would be ~decades, producing a garbage
	// drain/gain on the very first sample.
	t := 0.0
	if !s.lastUpdate.IsZero() {
		t = now.Sub(s.lastUpdate).Seconds()
	}
	// Base draw integrated over the interval is W*s (joules); the /3600 below
	// converts the whole `change` to Wh. The TX term is already an energy
	// (PerkByteTXWh is Wh per *kByte*), so convert bytes->kBytes and scale by
	// 3600 to keep it in joules until the shared /3600.
	txWh := float64(sumBytesOutThisTurn) / 1000.0 * float64(s.hw.EnergyConsumption.PerkByteTXWh)
	drain := float64(s.hw.EnergyConsumption.NormalPowerBaseW)*t + txWh*3600.0
	gain := 0.0
	if !s.InShadow {
		gain = float64(s.hw.BatteryChargeW) * t
	}
	change := float32(gain - drain)
	if s.batteryLevel >= s.hw.BatteryCapacityWh {
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

// SetInShadow updates the shadow flag under the state lock. Called from the
// MQTT update callback, concurrently with Update/Power.
func (s *NodeHwState) SetInShadow(v bool) {
	s.lock.Lock()
	s.InShadow = v
	s.lock.Unlock()
}

// Power returns a snapshot of the current battery state. Safe for concurrent use.
func (s *NodeHwState) Power() (batteryWh, capacityWh float32, inShadow, lowPower bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.hw == nil {
		return s.batteryLevel, 0, s.InShadow, false
	}
	low := s.hw.BatteryCapacityWh > 0 && s.batteryLevel <= s.hw.LowPowerThresholdWh
	return s.batteryLevel, s.hw.BatteryCapacityWh, s.InShadow, low
}

func (s *NodeHwState) format(change float32) string {
	var str string
	if s.hw.EnergyConsumption.LowPowerBaseW <= 0 || s.hw.BatteryCapacityWh <= 0 {
		str = "-"
	} else {
		lev := int(float64(s.batteryLevel) / float64(s.hw.BatteryCapacityWh) * 100.0)
		if lev >= 100 && change > 0 {
			change = 0
		}
		str = fmt.Sprintf("%d%%,%+.1fW %s", lev, change, goutils.BoolTo(s.InShadow, "shadow", "sun"))
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
