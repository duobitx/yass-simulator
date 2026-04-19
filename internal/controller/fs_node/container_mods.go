package fs_node

import (
	"fmt"

	v2 "github.com/duobitx/yass-operator/api/v1"
	"github.com/duobitx/yass-operator/internal/controller"
	"github.com/m-szalik/goutils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	experimentLogLevelVariableName = "EXPERIMENT_LOG_LEVEL"
)

type modFunc func(pod *v1.Pod, container *v1.Container)

func cmd(cmd ...string) modFunc {
	return func(_ *v1.Pod, container *v1.Container) {
		container.Command = cmd
	}
}

// FIXME: unused
// func rootFSReadOnly() modFunc {
// 	return func(_ *v1.Pod, container *v1.Container) {
// 		bvTrue := true
// 		if container.SecurityContext == nil {
// 			container.SecurityContext = &v1.SecurityContext{}
// 		}
// 		container.SecurityContext.ReadOnlyRootFilesystem = &bvTrue
// 	}
// }

func modMountSharedVolume(ro bool) modFunc {
	montPath := "/mnt/shared"
	return modComposite(
		modEnvsAppend(map[string]string{"SHARED_VOLUME_PATH": montPath}),
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
		if container.VolumeMounts == nil {
			container.VolumeMounts = vms
		} else {
			container.VolumeMounts = append(container.VolumeMounts, vms...)
		}
	}
}

// FIXME: unused
// func modFileProbes() modFunc {
// 	const filename = "/tmp/probe.txt"
// 	commands := []string{"/bin/ls", "-c", filename} // see internal-components->docker_tools
// 	fileProbe := &v1.Probe{
// 		ProbeHandler: v1.ProbeHandler{
// 			Exec: &v1.ExecAction{Command: commands},
// 		},
// 		InitialDelaySeconds: 8,
// 		TimeoutSeconds:      1,
// 		PeriodSeconds:       3,
// 		SuccessThreshold:    1,
// 		FailureThreshold:    2,
// 	}

// 	return modComposite(
// 		modEnvs(map[string]string{"PROBE_FILE": filename}),
// 		func(pod *v1.Pod, container *v1.Container) {
// 			container.ReadinessProbe = fileProbe.DeepCopy()
// 			container.LivenessProbe = fileProbe.DeepCopy()
// 		},
// 	)
// }

func modHttpProbes(port int) modFunc {
	const portName = "http-probe"
	fileProbe := &v1.Probe{
		ProbeHandler: v1.ProbeHandler{
			HTTPGet: &v1.HTTPGetAction{
				Path:   "/",
				Port:   intstr.IntOrString{Type: intstr.String, StrVal: portName},
				Scheme: "HTTP",
			},
		},
		InitialDelaySeconds: 8,
		TimeoutSeconds:      1,
		PeriodSeconds:       3,
		SuccessThreshold:    1,
		FailureThreshold:    2,
	}

	return func(pod *v1.Pod, container *v1.Container) {
		container.ReadinessProbe = fileProbe.DeepCopy()
		container.LivenessProbe = fileProbe.DeepCopy()
		if container.Ports == nil {
			container.Ports = []v1.ContainerPort{}
		}
		portFound := false
		for _, cp := range container.Ports {
			if cp.ContainerPort == int32(port) {
				portFound = true
				break
			}
		}
		if !portFound {
			container.Ports = append(container.Ports, v1.ContainerPort{
				Name:          portName,
				ContainerPort: int32(port),
				Protocol:      "TCP",
			})
		}
	}
}

func modEnvsAppend(vars map[string]string) modFunc {
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

func modCapability(capability v1.Capability) modFunc {
	return func(pod *v1.Pod, container *v1.Container) {
		if container.SecurityContext == nil {
			container.SecurityContext = &v1.SecurityContext{}
		}
		if container.SecurityContext.Capabilities == nil {
			container.SecurityContext.Capabilities = &v1.Capabilities{}
		}
		container.SecurityContext.Capabilities.Add = append(container.SecurityContext.Capabilities.Add, capability)
	}
}

func modLogLevelVariableSet() modFunc {
	ll := goutils.Env(experimentLogLevelVariableName, "")
	if ll != "" {
		return modEnvsAppend(map[string]string{"LOG_LEVEL": ll})
	}
	return func(pod *v1.Pod, container *v1.Container) {}
}

func modComposite(composites ...modFunc) modFunc {
	return func(pod *v1.Pod, container *v1.Container) {
		for _, c := range composites {
			c(pod, container)
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
	if len(simpleContainer.Envs) > 0 {
		modFunctions = append(modFunctions, modEnvsAppend(simpleContainer.Envs))
	}
	return modComposite(modFunctions...)
}
