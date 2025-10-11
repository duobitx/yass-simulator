package fs_node

import (
	"fmt"

	v2 "github.com/ESA-PhiLab/yass-operator/api/v1"
	"github.com/ESA-PhiLab/yass-operator/internal/controller"
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

func modMountSharedVolume(ro bool) modFunc {
	montPath := "/shared"
	return modComposite(
		modEnvs(map[string]string{"SHARED_VOLUME_PATH": montPath}),
		modVolumeMount(sharedVolumeName, montPath, ro),
	)
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

func modFileProbes(filename string) modFunc {
	fileProbe := &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			Exec: &v1.ExecAction{
				Command: []string{"/file_probe.sh"}, // see internal-components->docker_tools
			},
		},
		InitialDelaySeconds: 2,
		TimeoutSeconds:      1,
		PeriodSeconds:       3,
		SuccessThreshold:    1,
		FailureThreshold:    2,
	}

	return modComposite(
		modEnvs(map[string]string{"PROBE_FILE": filename}),
		func(pod *v1.Pod, container *v1.Container) {
			container.ReadinessProbe = fileProbe
			container.LivenessProbe = fileProbe
		},
	)
}

func modEnvs(vars map[string]string) modFunc {
	return func(pod *v1.Pod, container *v1.Container) {
		if container.Env == nil {
			container.Env = []v1.EnvVar{}
		}
		for k, v := range vars {
			container.Env = append(container.Env, v1.EnvVar{
				Name:  controller.NormalizeEnvName(k),
				Value: v,
			})
		}
	}
}

func modEnvFromField(name, fieldPath string) modFunc {
	return func(pod *v1.Pod, container *v1.Container) {
		if container.Env == nil {
			container.Env = []v1.EnvVar{}
		}
		container.Env = append(container.Env, v1.EnvVar{
			Name: controller.NormalizeEnvName(name),
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: fieldPath,
				},
			},
		})
	}
}

func modComposite(composites ...modFunc) modFunc {
	return func(pod *v1.Pod, container *v1.Container) {
		for _, c := range composites {
			c(pod, container)
		}
	}
}

func modResourcesLimit(resourceRequirements *v1.ResourceRequirements) modFunc {
	return func(pod *v1.Pod, container *v1.Container) {
		if resourceRequirements != nil {
			pod.Spec.Resources = resourceRequirements
		}
	}
}

func modFor(simpleContainer v2.SimpleContainer) modFunc {
	var modFunctions []modFunc
	if simpleContainer.ConfigurationFilesFromConfigMap != nil && simpleContainer.ConfigurationFilesFromConfigMap.ConfigMapRef != "" {
		mf := func(pod *v1.Pod, container *v1.Container) {
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
		modFunctions = append(modFunctions, mf)
	}
	if simpleContainer.Envs != nil && len(simpleContainer.Envs) > 0 {
		modFunctions = append(modFunctions, modEnvs(simpleContainer.Envs))
	}
	return modComposite(modFunctions...)
}
