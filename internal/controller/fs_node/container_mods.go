package fs_node

import v1 "k8s.io/api/core/v1"

type modFunc func(container *v1.Container)

func cmd(cmd ...string) modFunc {
	return func(container *v1.Container) {
		container.Command = cmd
	}
}

func rootFSReadOnly() modFunc {
	return func(container *v1.Container) {
		bvTrue := true
		if container.SecurityContext == nil {
			container.SecurityContext = &v1.SecurityContext{}
		}
		container.SecurityContext.ReadOnlyRootFilesystem = &bvTrue
	}
}

func modVolumeMount(volumeName, mountPoint string, ro bool) modFunc {
	return func(container *v1.Container) {
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
