package config

import (
	"fmt"

	"github.com/m-szalik/goutils"
	v1 "k8s.io/api/core/v1"
)

const (
	internalComponentsImage = "ghcr.io/duobitx/yass-internal-components"
	latest                  = "latest"
)

type Configuration struct {
	InternalComponentImage           string
	InternalComponentImagePullPolicy v1.PullPolicy
	DisableNetworkingManipulation    bool
	MessagingBrokerHostPort          string
	// ObservabilityLokiURL / ObservabilityPrometheusURL are the cluster-internal
	// base URLs used to delete an experiment's data when the experiment is
	// deleted. An empty value disables cleanup for that backend. Loki/Prometheus
	// being absent or unreachable must never crash or block the operator
	// (best-effort cleanup).
	ObservabilityLokiURL       string
	ObservabilityPrometheusURL string
	// ObservabilityLokiTenant is the optional Loki X-Scope-OrgID tenant header.
	ObservabilityLokiTenant string
}

func NewConfiguration() (*Configuration, error) {
	imageVersion := goutils.Env("INTERNAL_COMPONENTS_VERSION", latest)
	imagePullPolicy := v1.PullIfNotPresent
	if imageVersion == latest {
		imagePullPolicy = v1.PullAlways
	}
	return &Configuration{
		InternalComponentImage:           fmt.Sprintf("%s:%s", internalComponentsImage, imageVersion),
		InternalComponentImagePullPolicy: imagePullPolicy,
		DisableNetworkingManipulation:    goutils.Env("DISABLE_NETWORKING_MANIPULATION", false),
		MessagingBrokerHostPort:          goutils.Env("MESSAGING_BROKER_HOST_PORT", "messaging:1883"),
		ObservabilityLokiURL:             goutils.Env("LOKI_URL", "http://loki.yass-system.svc.cluster.local:3100"),
		ObservabilityPrometheusURL:       goutils.Env("PROMETHEUS_URL", "http://prometheus.yass-system.svc.cluster.local:9090"),
		ObservabilityLokiTenant:          goutils.Env("LOKI_TENANT", ""),
	}, nil
}
