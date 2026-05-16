module github.com/duobitx/yass-simulator/internal-components/metrics-bridge

go 1.25.7

require (
	github.com/duobitx/yass-simulator/internal-components/go-common v0.0.0-20251214230811-f06efdbb16cf
	github.com/m-szalik/com-facade v0.0.0-20260419000112-a6e0e334d5ca
	github.com/m-szalik/goutils v0.4.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/eclipse/paho.mqtt.golang v1.5.1 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/common v0.67.4 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

replace github.com/duobitx/yass-simulator/internal-components/go-common => ../go-common
