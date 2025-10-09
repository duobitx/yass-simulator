package fs_node

import (
	"fmt"

	v2 "github.com/ESA-PhiLab/yass-operator/api/v1"
	v1 "k8s.io/api/core/v1"
)

type modFunc func(pod *v1.Pod, container *v1.Container)

func cmd(cmd ...string) modFunc {
	return func(_ *v1.Pod, container *v1.Container) {
		container.Command = cmd
	}
}

func rootFSReadOnly() modFunc {
	return func(_ *v1.Pod, container *v1.Container) {
		bvTrue := true
		if container.SecurityContext == nil {
			container.SecurityContext = &v1.SecurityContext{}
		}
		container.SecurityContext.ReadOnlyRootFilesystem = &bvTrue
	}
}

func modVolumeMount(volumeName, mountPoint string, ro bool) modFunc {
	return func(_ *v1.Pod, container *v1.Container) {
		vms := []v1.VolumeMount{
			{
				Name:      volumeName,
				ReadOnly:  ro,
				MountPath: mountPoint,
			},
		}
		if container.VolumeDevices == nil {
			container.VolumeMounts = vms
		} else {
			container.VolumeMounts = append(container.VolumeMounts, vms...)
		}
	}
}

func modFor(simpleContainer v2.SimpleContainer) modFunc {
	return func(pod *v1.Pod, container *v1.Container) {
		if simpleContainer.ConfigurationFilesFromConfigMap != nil && simpleContainer.ConfigurationFilesFromConfigMap.ConfigMapRef != "" {
			volName := fmt.Sprintf("vol-%s", simpleContainer.ConfigurationFilesFromConfigMap.ConfigMapRef)
			valFalse := false
			if pod.Spec.Volumes == nil {
				pod.Spec.Volumes = []v1.Volume{}
			}
			volFound := false
			for _, vol := range pod.Spec.Volumes {
				if vol.Name == volName {
					volFound = true
					break
				}
			}
			if !volFound {
				pod.Spec.Volumes = append(pod.Spec.Volumes, v1.Volume{
					Name: volName,
					VolumeSource: v1.VolumeSource{
						ConfigMap: &v1.ConfigMapVolumeSource{
							LocalObjectReference: v1.LocalObjectReference{Name: simpleContainer.ConfigurationFilesFromConfigMap.ConfigMapRef},
							Optional:             &valFalse,
						},
					},
				})
			}

			if container.VolumeMounts == nil {
				container.VolumeMounts = []v1.VolumeMount{}
			}
			container.VolumeMounts = append(container.VolumeMounts, v1.VolumeMount{
				Name:      volName,
				ReadOnly:  true,
				MountPath: simpleContainer.ConfigurationFilesFromConfigMap.MountPath,
			})
		}
	}
}
