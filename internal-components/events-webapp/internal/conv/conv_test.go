package conv

import (
	"testing"
	"time"

	"github.com/duobitx/yass-simulator/internal-components/events-webapp/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestGeoUpdateConv(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		content string
		want    any
		wantErr bool
	}{
		{
			name:    "FsNodeUpdateConv",
			topic:   "updates/sat1",
			content: `{"name":"new-norcia","pos_str":"lat=-30.33,lng=116.85","x":-620.31165,"y":-5474.5703,"z":-3202.4648,"lat":-30.334846,"lng":116.85053,"network_params":[{"ip":"10.244.0.20","distance":2475.0374,"package_delay":0.009250125,"package_loss":0.1,"subject":"yaogan-25c"}],"updated_unix_millis":1765685662000}`,
			want: api.PositionEvent{
				BaseEvent: api.BaseEvent{
					Source:    "new-norcia",
					Timestamp: time.UnixMilli(1765685662000),
					EventType: "PositionEvent",
				},
				X:   -620.31165,
				Y:   -5474.5703,
				Z:   -3202.4648,
				Lat: -30.334846,
				Lng: 116.85053,
				Alt: 0,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FsNodeUpdateConv(tt.topic, []byte(tt.content))
			if (err != nil) != tt.wantErr {
				t.Errorf("FsNodeUpdateConv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
