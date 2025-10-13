package config

import (
	"strings"

	"github.com/m-szalik/goutils"
	v1 "k8s.io/api/core/v1"
)

type Configuration struct {
	InternalComponentImage           string
	InternalComponentImagePullPolicy v1.PullPolicy
}

func NewConfiguration() (*Configuration, error) {
	image := goutils.Env("INTERNAL_COMPONENTS_IMAGE", "ghcr.io/esa-philab/yass/internal-components:latest")
	imagePullPolicy := v1.PullIfNotPresent
	if !strings.Contains(image, ":") || strings.HasSuffix(image, "latest") {
		imagePullPolicy = v1.PullAlways
	}
	return &Configuration{
		InternalComponentImage:           image,
		InternalComponentImagePullPolicy: imagePullPolicy,
	}, nil
}
