/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fs_node

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ESA-PhiLab/yass-operator/internal/controller"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	yassv1 "github.com/ESA-PhiLab/yass-operator/api/v1"
)

const sharedVolumeName = "fs-node-shared-volume"
const engineVolumeName = "engine-volume"
const agentTMPVolumeName = "agent-tmp-volume"

// FsNodeReconciler reconciles a FsNode object
type FsNodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type containerSpec struct {
	name      string
	image     string
	resources v1.ResourceRequirements
	extraEnv  map[string]string
	ports     []v1.ContainerPort
	mods      []modFunc
}

// +kubebuilder:rbac:groups=yass.int.esa.yass,resources=fsnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=yass.int.esa.yass,resources=fsnodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=yass.int.esa.yass,resources=fsnodes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the FsNode object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *FsNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	var fsNode yassv1.FsNode
	err := r.Get(ctx, req.NamespacedName, &fsNode)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// 🔴 Resource was deleted
			logger.Info("FsNode deleted", "name", req.NamespacedName)
			err := r.removeFsNode(ctx, req)
			return ctrl.Result{}, err
		}
		// Some other error
		return ctrl.Result{}, err
	} else {
		experimentName := fsNode.Labels[controller.LabelExperiment]
		if experimentName == "" {
			err := fmt.Errorf("experiment label (%s) not set", controller.LabelExperiment)
			_ = r.updateStatusCondition(ctx, &fsNode, "ExperimentAssigned", "no experiment label", err)
			return ctrl.Result{}, err
		}

		requeue, err := r.updateHardwareSpec(ctx, &fsNode)
		if requeue || err != nil {
			_ = r.updateStatusCondition(ctx, &fsNode, "hardwareSpec", "resolving hardwareSpec", err)
		}
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "cannot update hardwareSpec")
		}
		if requeue {
			return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
		}
		err = r.createOrUpdateFsNodePod(ctx, req, &fsNode)
		if err != nil {
			_ = r.updateStatusCondition(ctx, &fsNode, "podCreation", "pod", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FsNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&yassv1.FsNode{}).
		Named("fsNode").
		Complete(r)
}

func (r *FsNodeReconciler) removeFsNode(ctx context.Context, req ctrl.Request) error {
	pod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: createPodName(req)}, pod)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.Delete(ctx, pod)
}

func (r *FsNodeReconciler) createOrUpdateFsNodePod(ctx context.Context, req ctrl.Request, fsNode *yassv1.FsNode) error {
	commonComponentsImage := "ghcr.io/esa-philab/yass/internal-components:latest"
	podName := createPodName(req)
	pod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: podName}, pod)
	if apierrors.IsNotFound(err) {
		experimentName := fsNode.Labels[controller.LabelExperiment]
		// create Pod
		enginePorts := []v1.ContainerPort{}
		for port := 3000; port <= 3020; port++ {
			enginePorts = append(enginePorts, v1.ContainerPort{ContainerPort: int32(port)})
		}
		enginePorts = append(enginePorts, v1.ContainerPort{ContainerPort: int32(8080)})
		agentParameters, err := fsNode.Spec.Agent.ParametersAsMap("agent")
		if err != nil {
			return err
		}
		engineParameters, err := fsNode.Spec.Engine.ParametersAsMap("engine")
		if err != nil {
			return err
		}
		engineResources := v1.ResourceRequirements{Limits: map[v1.ResourceName]resource.Quantity{}}
		if fsNode.Spec.HardwareSpec != nil {
			if fsNode.Spec.HardwareSpec.CPU != nil && !fsNode.Spec.HardwareSpec.CPU.IsZero() {
				engineResources.Limits[v1.ResourceCPU] = *fsNode.Spec.HardwareSpec.CPU
			}
			if fsNode.Spec.HardwareSpec.Memory != nil && !fsNode.Spec.HardwareSpec.Memory.IsZero() {
				engineResources.Limits[v1.ResourceMemory] = *fsNode.Spec.HardwareSpec.Memory
			}
		}
		var containers []v1.Container
		containerSpecs := []containerSpec{
			{
				name:  "world-controller",
				image: commonComponentsImage,
				mods: []modFunc{
					modVolumeMount(agentTMPVolumeName, "/tmp", false),
					cmd("/world-controller"),
				},
			},
			{
				name:      "agent",
				image:     fsNode.Spec.Agent.Image,
				resources: v1.ResourceRequirements{},
				extraEnv:  agentParameters,
				ports:     nil,
				mods: []modFunc{
					modVolumeMount(agentTMPVolumeName, "/tmp", false),
				},
			},
			{
				name:      "engine",
				image:     fsNode.Spec.Engine.Image,
				resources: engineResources,
				extraEnv:  engineParameters,
				ports:     enginePorts,
				mods: []modFunc{
					modVolumeMount(engineVolumeName, "/var/data", false),
					rootFSReadOnly(),
				},
			},
			{
				name:  "engine-gw",
				image: "ghcr.io/esa-philab/yass/gateway:latest",
				extraEnv: map[string]string{
					"HANDLERS": "http:messaging-gw:8080;http:engine:8080",
				},
				ports: []v1.ContainerPort{{ContainerPort: 8080, Protocol: "TCP"}},
			},
		}
		for _, cs := range containerSpecs {
			container, err := r.createFsNodeContainerTemplate(fsNode, cs)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("cannot create container template %s", cs.name))
			}
			if container == nil {
				return fmt.Errorf("cannot create container template %s, nil returned without error", cs.name)
			}
			if cs.mods != nil {
				for _, mod := range cs.mods {
					mod(container)
				}
			}
			containers = append(containers, *container)
		}

		var diskSizeLimit *resource.Quantity = nil
		if fsNode.Spec.HardwareSpec != nil {
			diskSizeLimit = fsNode.Spec.HardwareSpec.DiskSpace
		}
		pod = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: fsNode.Namespace,
				Labels: map[string]string{
					controller.LabelFsNode:     fsNode.Name,
					controller.LabelExperiment: experimentName,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(fsNode, v1.SchemeGroupVersion.WithKind("FsNode")),
				},
			},
			Spec: v1.PodSpec{
				Volumes: []v1.Volume{
					{Name: sharedVolumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}, // mounted by default under /shared
					{Name: engineVolumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{SizeLimit: diskSizeLimit}}},
					{Name: agentTMPVolumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
				},
				Containers:         containers,
				ServiceAccountName: controller.ServiceAccountName,
			},
		}

		err = r.Create(ctx, pod)
		_ = r.updateStatusCondition(ctx, fsNode, "PodCreation", "creation", err)
		if err != nil {
			return err
		}
	}
	return nil
}

func createPodName(req ctrl.Request) string {
	return fmt.Sprintf("%s-pod", req.Name)
}

func (r *FsNodeReconciler) createFsNodeContainerTemplate(fsNode *yassv1.FsNode, cs containerSpec) (*v1.Container, error) {
	experimentName := fsNode.Labels[controller.LabelExperiment]
	envVars := []v1.EnvVar{
		{Name: controller.LabelFsNode, Value: fsNode.Name},
		{Name: controller.LabelExperiment, Value: experimentName},
	}
	for k, v := range cs.extraEnv {
		envVars = append(envVars, v1.EnvVar{Name: k, Value: v})
	}
	for _, ev := range envVars {
		ev.Name = strings.ToUpper(strings.ReplaceAll(ev.Name, "-", "_"))
	}
	container := v1.Container{
		Name:      cs.name,
		Image:     cs.image,
		Ports:     cs.ports,
		Env:       envVars,
		Resources: cs.resources,
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      sharedVolumeName,
				ReadOnly:  false,
				MountPath: "/shared",
			},
		},
		LivenessProbe:   nil,
		ReadinessProbe:  nil,
		ImagePullPolicy: "Always",
	}
	return &container, nil
}

func (r *FsNodeReconciler) updateHardwareSpec(ctx context.Context, fsNode *yassv1.FsNode) (bool, error) {
	if fsNode.Spec.HardwareSpec == nil && fsNode.Spec.HardwareSpecRef != "" {
		hardwareDef := &yassv1.HardwareDefinition{}
		err := r.Get(ctx, types.NamespacedName{Name: fsNode.Spec.HardwareSpecRef}, hardwareDef)
		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf("HardwareSpecRef '%s' not found", fsNode.Spec.HardwareSpecRef)
		}
		if err != nil {
			return false, errors.Wrap(err, "cannot fetch HardwareSpecRef")
		}
		if hardwareDef.Spec != nil {
			hwSpecCopy := hardwareDef.Spec.DeepCopy()
			fsNode.Spec.HardwareSpec = hwSpecCopy
			err = r.Update(ctx, fsNode)
			if err != nil {
				return false, err
			}
		}
		return true, nil
	}
	return false, nil
}

func (r *FsNodeReconciler) updateStatusCondition(ctx context.Context, fsNode *yassv1.FsNode, ctype string, reason string, cause error) error {
	if reason == "" || cause == nil {
		reason = "ok"
	}
	var condition *metav1.Condition
	found := false
	for _, c := range fsNode.Status.Conditions {
		if c.Type == ctype {
			condition = &c
			found = true
			break
		}
	}
	if !found {
		condition = &metav1.Condition{
			Type: ctype,
		}
	}
	if cause == nil {
		condition.Status = metav1.ConditionTrue
		condition.Message = ""
	} else {
		condition.Status = metav1.ConditionFalse
		condition.Message = fmt.Sprintf("error: %s", cause.Error())
	}
	condition.Reason = strings.ReplaceAll(reason, " ", "_")
	condition.LastTransitionTime = metav1.Time{Time: time.Now()}
	if !found {
		fsNode.Status.Conditions = append(fsNode.Status.Conditions, *condition)
	}
	err := r.Status().Update(ctx, fsNode)
	if err != nil {
		logf.FromContext(ctx).Error(err, fmt.Sprintf("cannot update fsNode %s status", fsNode.Name))
	}
	return cause
}
