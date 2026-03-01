module github.com/duobitx/yass-internal-components/events-webapp

go 1.25

toolchain go1.25.4

require (
	github.com/duobitx/yass-internal-components/go-common v0.0.0-20251130234904-36bc4d78cc85
	github.com/m-szalik/goutils v0.2.1
	github.com/stretchr/testify v1.11.1
	k8s.io/apimachinery v0.34.2
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/eclipse/paho.mqtt.golang v1.5.1 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sync v0.18.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
)

replace github.com/duobitx/yass-internal-components/go-common => ../go-common
