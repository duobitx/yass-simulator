module github.com/duobitx/yass-simulator/internal-components/events-webapp

go 1.25.7

require (
	github.com/duobitx/yass-simulator/internal-components/go-common v0.0.0-20251214230811-f06efdbb16cf
	github.com/m-szalik/com-facade v0.0.0-20260419000112-a6e0e334d5ca
	github.com/m-szalik/goutils v0.4.0
	github.com/stretchr/testify v1.11.1
	k8s.io/apimachinery v0.35.3
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/eclipse/paho.mqtt.golang v1.5.1 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
)

replace github.com/duobitx/yass-simulator/internal-components/go-common => ../go-common
