package hw

import (
	"fmt"
	"testing"

	yassv1 "github.com/duobitx/yass-operator/api/v1"
	"github.com/stretchr/testify/assert"
)

func TestNodeHwState_format(t *testing.T) {
	tests := []struct {
		batteryLevel float32
		batteryCap   float32
		change       float32
		shadow       bool
		want         string
	}{
		{
			batteryLevel: 51,
			batteryCap:   100,
			change:       0.1,
			want:         "51%,+0.1W ☀",
			shadow:       false,
		},
		{
			batteryLevel: 100,
			batteryCap:   100,
			change:       -0.1,
			want:         "100%,-0.1W",
			shadow:       true,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("Expect %s", tt.want), func(t *testing.T) {
			s := &NodeHwState{
				hw: &yassv1.HardwareSpec{
					BatteryCapacityWh: tt.batteryCap,
				},
				InShadow:     tt.shadow,
				batteryLevel: tt.batteryLevel,
			}
			assert.Equal(t, tt.want, s.format(tt.change))
		})
	}
}
