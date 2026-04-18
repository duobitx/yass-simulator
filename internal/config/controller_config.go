package config

import (
	"fmt"

	"github.com/m-szalik/goutils"
	v1 "k8s.io/api/core/v1"
)

const (
	internalComponentsImage = "ghcr.io/duobitx/yass/internal-components"
	latest                  = "latest"
)

type Configuration struct {
	InternalComponentImage           string
	InternalComponentImagePullPolicy v1.PullPolicy
	DisableNetworkingManipulation    bool
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
	}, nil
}
