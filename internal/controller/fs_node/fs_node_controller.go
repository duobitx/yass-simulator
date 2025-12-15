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

	"github.com/duobitx/yass-operator/internal/config"
	"github.com/duobitx/yass-operator/internal/controller"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	"gopkg.in/inf.v0"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	yassv1 "github.com/duobitx/yass-operator/api/v1"
)

const (
	engineVolumeName                = "engine-volume"
	agentTMPVolumeName              = "agent-tmp-volume"
	sharedVolumeName                = "fs-node-shared-volume"
	removeFsNodeComponentsFinalizer = "fsnode-controller/cleanup"
)

var engineOpenPorts = map[int]v1.Protocol{
	3000: v1.ProtocolTCP, 3001: v1.ProtocolTCP, 3002: v1.ProtocolTCP, 3003: v1.ProtocolTCP, 3004: v1.ProtocolTCP, 3005: v1.ProtocolTCP, 3006: v1.ProtocolTCP,
	3007: v1.ProtocolTCP, 3008: v1.ProtocolTCP, 3009: v1.ProtocolTCP, 3010: v1.ProtocolTCP,
	3011: v1.ProtocolUDP, 3012: v1.ProtocolUDP, 3013: v1.ProtocolUDP, 3014: v1.ProtocolUDP, 3015: v1.ProtocolUDP,
}

// FsNodeReconciler reconciles a FsNode object
type FsNodeReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Configuration *config.Configuration
}

type containerSpec struct {
	name  string
	image string
	ports []v1.ContainerPort
	mods  []modFunc
}

// + kubebuilder:rbac:groups=yass.int.esa.yass,resources=fsnodes,verbs=get;list;watch;create;update;patch;delete
// + kubebuilder:rbac:groups=yass.int.esa.yass,resources=fsnodes/status,verbs=get;update;patch
// + kubebuilder:rbac:groups=yass.int.esa.yass,resources=fsnodes/finalizers,verbs=update
// +kubebuilder:rbac:groups=*,resources=*,verbs=*
// TODO limit permissions

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
			return ctrl.Result{}, nil // ignore not found
		}
		return ctrl.Result{}, err
	}

	if !fsNode.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(&fsNode, removeFsNodeComponentsFinalizer) {
			// Run cleanup logic
			logger.Info("FsNode deleted", "name", req.NamespacedName)
			err = r.removeFsNode(ctx, req)
			if err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&fsNode, removeFsNodeComponentsFinalizer)
			if err := r.Update(ctx, &fsNode); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&fsNode, removeFsNodeComponentsFinalizer) {
		controllerutil.AddFinalizer(&fsNode, removeFsNodeComponentsFinalizer)
		if err := r.Update(ctx, &fsNode); err != nil {
			return ctrl.Result{}, err
		}
	}

	requeue, err := r.updateHardwareSpec(ctx, &fsNode)
	if requeue {
		return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
	}
	defer func() {
		fsNode.Status.Ready = goutils.AllMatch(fsNode.Status.Conditions, func(element *metav1.Condition) bool {
			return element.Status == metav1.ConditionTrue
		})
		err := r.Status().Update(ctx, &fsNode)
		if err != nil {
			logger.Error(err, "error updating fsNode status")
		}
	}()
	if err != nil {
		r.updateStatusCondition(&fsNode, "hardwareSpec", err.Error(), "hardwareNotAssigned", false)
	} else {
		r.updateStatusCondition(&fsNode, "hardwareSpec", "", "hardwareAssigned", true)
	}
	experimentName := fsNode.Labels[controller.LabelExperiment]
	if experimentName == "" {
		r.updateStatusCondition(&fsNode, "experimentAssigned", fmt.Sprintf("no %s label found", controller.LabelExperiment), "noExperimentLabel", false)
		return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
	}
	r.updateStatusCondition(&fsNode, "experimentAssigned", "experiment assigned", "experimentLabelFound", true)
	err = r.createOrUpdateFsNodePod(ctx, &fsNode)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.createOrUpdateFsNodeService(ctx, &fsNode)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *FsNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&yassv1.FsNode{}).
		Named("fsNode-controller").
		Owns(&v1.Pod{}).
		Complete(r)
}

func (r *FsNodeReconciler) removeFsNode(ctx context.Context, req ctrl.Request) error {
	pod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, pod)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.Delete(ctx, pod)
}

func (r *FsNodeReconciler) createOrUpdateFsNodePod(ctx context.Context, fsNode *yassv1.FsNode) error {
	const ctype = "fsNodePod"
	True := true
	podName := fsNode.Name
	pod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Namespace: fsNode.Namespace, Name: podName}, pod)
	if err == nil || !apierrors.IsNotFound(err) { // already exists OR other unexpected error
		r.updateStatusConditionForObject(fsNode, ctype, pod, err)
		return err
	}
	// POD not found we need to create the POD
	experimentName := fsNode.Labels[controller.LabelExperiment]
	// create Pod

	// FIXME: commented as _enginePorts are not used
	// var _enginePorts []v1.ContainerPort
	// for port, prot := range engineOpenPorts {
	// 	_enginePorts = append(_enginePorts, v1.ContainerPort{ContainerPort: int32(port), Protocol: prot})
	// }
	// _enginePorts = append(_enginePorts, v1.ContainerPort{ContainerPort: int32(8080)})

	engineResources := v1.ResourceRequirements{Limits: map[v1.ResourceName]resource.Quantity{}}
	if fsNode.Spec.HardwareSpec != nil {
		if fsNode.Spec.HardwareSpec.CPU != nil && !fsNode.Spec.HardwareSpec.CPU.IsZero() {
			engineResources.Limits[v1.ResourceCPU] = *fsNode.Spec.HardwareSpec.CPU
		}
		if fsNode.Spec.HardwareSpec.Memory != nil && !fsNode.Spec.HardwareSpec.Memory.IsZero() {
			engineResources.Limits[v1.ResourceMemory] = *fsNode.Spec.HardwareSpec.Memory
		}
	}
	// TODO mount
	var diskSizeLimit *resource.Quantity = nil
	terminationGracePeriodSeconds := int64(8)
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
			TerminationGracePeriodSeconds: &terminationGracePeriodSeconds,
			ServiceAccountName:            "yass-experiment-sa",
		},
	}

	initContainer := &v1.Container{
		Name:            "resource-to-json-fsnode",
		Command:         []string{"/resource-to-json"},
		Image:           r.Configuration.InternalComponentImage,
		ImagePullPolicy: r.Configuration.InternalComponentImagePullPolicy,
	}
	modMountSharedVolume(false)(pod, initContainer)
	modEnvs(map[string]string{"DST_FILE": "/mnt/shared/fs-node.json", "RESOURCE_KIND": fsNode.Kind, "RESOURCE_NAME": fsNode.Name})(pod, initContainer)
	modEnvFromField("NAMESPACE", "metadata.namespace")(pod, initContainer)
	pod.Spec.InitContainers = []v1.Container{*initContainer}

	containers, err := r.getSystemContainers(fsNode, pod)
	if err != nil {
		return err
	}
	engineContainers := r.getEngineContainers(fsNode)
	pod.Spec.Containers = append(containers, engineContainers...)
	pod.Spec.AutomountServiceAccountToken = &True

	for _, volume := range fsNode.Spec.EngineVolumes {
		if volume.ConfigMap != nil || volume.Secret != nil {
			pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
		} else {
			return fmt.Errorf("invalid volume %s, allowed ConfigMap or Secret type only", volume.Name)
		}
	}
	err = r.Create(ctx, pod)
	if apierrors.IsAlreadyExists(err) {
		err = nil
	}
	r.updateStatusConditionForObject(fsNode, ctype, pod, err)
	return err
}

// System containers for the new FSNode pod.
func (r *FsNodeReconciler) getSystemContainers(fsNode *yassv1.FsNode, pod *v1.Pod) ([]v1.Container, error) {
	containers := make([]v1.Container, 0)
	containerSpecs := []containerSpec{
		{
			name:  "world-controller",
			image: r.Configuration.InternalComponentImage,
			mods: []modFunc{
				modLogLevelVariableSet(),
				cmd("/world-controller-wrapper.sh"),
				modHttpProbes(8801),
				modEnvFromField("POD_IP", "status.podIP"),
				modEnvFromField("NAMESPACE", "metadata.namespace"),
				modEnvs(map[string]string{"RESOURCE_NAME": fsNode.Name}),
				modMountSharedVolume(false),
				modCapability("NET_ADMIN"),
			},
		},
		{
			name:  "agent",
			image: fsNode.Spec.Agent.Image,
			ports: nil,
			mods: []modFunc{
				modLogLevelVariableSet(),
				modVolumeMount(agentTMPVolumeName, "/tmp", false),
				modFor(fsNode.Spec.Agent),
				modMountSharedVolume(true),
			},
		},
	}
	for _, cs := range containerSpecs {
		container := r.createFsNodeContainerTemplate(fsNode, cs)
		if container == nil {
			return nil, fmt.Errorf("cannot create container template %s, nil returned without error", cs.name)
		}
		if cs.mods != nil {
			for _, mod := range cs.mods {
				mod(pod, container)
			}
		}
		containers = append(containers, *container)
	}
	return containers, nil
}

// Engine containers for the new FSNode pod.
func (r *FsNodeReconciler) getEngineContainers(fsNode *yassv1.FsNode) []v1.Container {
	var ecResourceRequirements *v1.ResourceRequirements
	divider := len(fsNode.Spec.EngineContainers)
	if divider > 0 {
		if fsNode.Spec.HardwareSpec != nil {
			ecResourceRequirements = &v1.ResourceRequirements{Limits: v1.ResourceList{}}
			cpu := divideQuantityByInt(fsNode.Spec.HardwareSpec.CPU, int64(divider))
			if cpu != nil {
				ecResourceRequirements.Limits[v1.ResourceCPU] = *cpu
			}
			mem := divideQuantityByInt(fsNode.Spec.HardwareSpec.Memory, int64(divider))
			if cpu != nil {
				ecResourceRequirements.Limits[v1.ResourceMemory] = *mem
			}
		}
	}

	experimentLogLevel := goutils.Env(experimentLogLevelVariableName, "")
	var engineContainers []v1.Container
	for _, engineContainer := range fsNode.Spec.EngineContainers {
		eContainer := engineContainer.DeepCopy()
		if ecResourceRequirements != nil {
			eContainer.Resources = *ecResourceRequirements
		}
		if eContainer.VolumeMounts == nil {
			eContainer.VolumeMounts = []v1.VolumeMount{}
		}
		eContainer.VolumeMounts = append(eContainer.VolumeMounts, v1.VolumeMount{
			Name:      engineVolumeName,
			MountPath: "/mnt/engine",
			ReadOnly:  false,
		})
		setVariableIfUnset(eContainer, "LOG_LEVEL", experimentLogLevel)
		engineContainers = append(engineContainers, *eContainer)
	}

	return engineContainers
}

func (r *FsNodeReconciler) createOrUpdateFsNodeService(ctx context.Context, fsNode *yassv1.FsNode) error {
	const ctype = "fsNodeService"
	fsNodeService := &v1.Service{}
	objKey := client.ObjectKey{
		Namespace: fsNode.Namespace,
		Name:      fsNode.Name,
	}
	err := r.Get(ctx, objKey, fsNodeService)
	if err == nil { // exists
		r.updateStatusConditionForObject(fsNode, ctype, fsNodeService, nil)
		return nil
	}
	if !apierrors.IsNotFound(err) { // unexpected error
		r.updateStatusConditionForObject(fsNode, ctype, fsNodeService, err)
		return err
	}
	// not found -> create
	labels := map[string]string{
		controller.LabelFsNode:     fsNode.Name,
		controller.LabelExperiment: fsNode.Labels[controller.LabelExperiment],
	}
	_engineServicePorts := make([]v1.ServicePort, 0)
	for port, proto := range engineOpenPorts {
		_engineServicePorts = append(_engineServicePorts, v1.ServicePort{
			Name:       strings.ToLower(fmt.Sprintf("port%d%s", port, proto)),
			Protocol:   proto,
			Port:       int32(port),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: int32(port)},
		})
	}
	fsNodeService = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fsNode.Name,
			Namespace: fsNode.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(fsNode, v1.SchemeGroupVersion.WithKind("FsNode")),
			},
		},
		Spec: v1.ServiceSpec{
			Selector:  labels,
			Ports:     _engineServicePorts,
			ClusterIP: "None", //  # headless service - no virtual IP
		},
	}
	err = r.Create(ctx, fsNodeService)
	if apierrors.IsAlreadyExists(err) {
		err = nil
	}
	r.updateStatusConditionForObject(fsNode, ctype, fsNodeService, err)
	return err
}

func (r *FsNodeReconciler) createFsNodeContainerTemplate(fsNode *yassv1.FsNode, cs containerSpec) *v1.Container {
	experimentName := fsNode.Labels[controller.LabelExperiment]
	envVars := []v1.EnvVar{
		{Name: controller.NormalizeEnvName(controller.LabelFsNode), Value: fsNode.Name},
		{Name: controller.NormalizeEnvName(controller.LabelExperiment), Value: experimentName},
	}
	container := v1.Container{
		Name:            cs.name,
		Image:           cs.image,
		Ports:           cs.ports,
		Env:             envVars,
		VolumeMounts:    []v1.VolumeMount{},
		LivenessProbe:   nil,
		ReadinessProbe:  nil,
		ImagePullPolicy: r.Configuration.InternalComponentImagePullPolicy,
	}
	return &container
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
			return true, nil
		}
	}
	return false, nil
}

func (r *FsNodeReconciler) updateStatusConditionForObject(fsNode *yassv1.FsNode, ctype string, obj client.Object, cause error) {
	newReason := ""
	newStatus := false
	newMessage := ""
	if cause != nil {
		newMessage = cause.Error()
		newReason = "error"
	} else {
		newMessage = ""
		switch x := obj.(type) {
		case *v1.Pod:
			newStatus = x.Status.Phase == v1.PodRunning || x.Status.Phase == v1.PodSucceeded
			newReason = string(x.Status.Phase)
		default:
			newStatus = true
			newReason = "ok"
		}
	}
	r.updateStatusCondition(fsNode, ctype, newMessage, newReason, newStatus)
}

func (r *FsNodeReconciler) updateStatusCondition(fsNode *yassv1.FsNode, ctype, message, reason string, status bool) {
	var condition *metav1.Condition
	found := false
	for _, c := range fsNode.Status.Conditions {
		if c.Type == ctype {
			condition = c
			found = true
			break
		}
	}
	if !found {
		condition = &metav1.Condition{
			Type:   ctype,
			Status: metav1.ConditionUnknown,
			Reason: "undefined",
		}
	}
	newStatus := goutils.BoolTo(status, metav1.ConditionTrue, metav1.ConditionFalse)
	if condition.Status != newStatus || condition.Reason != reason || condition.Message != message {
		condition.LastTransitionTime = metav1.Time{Time: time.Now()}
		condition.Status = newStatus
		condition.Message = message
		condition.Reason = reason
	}
	if !found {
		fsNode.Status.Conditions = append(fsNode.Status.Conditions, condition)
	}
}

func divideQuantityByInt(q *resource.Quantity, n int64) *resource.Quantity {
	if n == 0 {
		panic("divide by zero")
	}
	if q == nil {
		return nil
	}
	const scale = 3          // 0 for integer, 3 for milli, etc.
	qDec := q.AsDec()        // *inf.Dec (this may promote internally)
	nDec := inf.NewDec(n, 0) // integer divisor
	out := new(inf.Dec).QuoRound(qDec, nDec, scale, inf.RoundHalfUp)
	if out == nil {
		return nil
	}
	return resource.NewDecimalQuantity(*out, resource.DecimalSI)
}

func setVariableIfUnset(container *v1.Container, variableName, variableValue string) {
	if variableValue == "" {
		return
	}
	for _, v := range container.Env {
		if v.Name == variableName && v.Value != "" {
			return
		}
	}
	container.Env = append(container.Env, v1.EnvVar{
		Name:  variableName,
		Value: variableValue,
	})
}
