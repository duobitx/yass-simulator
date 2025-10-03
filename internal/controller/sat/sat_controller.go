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

package sat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ESA-PhiLab/yass-experiment-operator/internal/controller"
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

	yassv1 "github.com/ESA-PhiLab/yass-experiment-operator/api/v1"
)

const sharedVolumeName = "sat-shared-volume"
const engineVolumeName = "engine-volume"

// SatReconciler reconciles a Sat object
type SatReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type containerSpec struct {
	name      string
	image     string
	resources v1.ResourceRequirements
	extraEnv  map[string]string
	ports     []v1.ContainerPort
}

// +kubebuilder:rbac:groups=yass.int.esa.yass,resources=sats,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=yass.int.esa.yass,resources=sats/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=yass.int.esa.yass,resources=sats/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Sat object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *SatReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	var sat yassv1.Sat
	err := r.Get(ctx, req.NamespacedName, &sat)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// 🔴 Resource was deleted
			logger.Info("Sat deleted", "name", req.NamespacedName)
			err := r.removeSatellite(ctx, req)
			return ctrl.Result{}, err
		}
		// Some other error
		return ctrl.Result{}, err
	} else {
		experimentName := sat.Labels[controller.LabelExperiment]
		if experimentName == "" {
			err := fmt.Errorf("experiment label (%s) not set", controller.LabelExperiment)
			_ = r.updateStatusCondition(ctx, &sat, "ExperimentAssigned", "no experiment label", err)
			return ctrl.Result{}, err
		}

		requeue, err := r.updateHardwareSpec(ctx, &sat)
		if requeue || err != nil {
			_ = r.updateStatusCondition(ctx, &sat, "hardwareSpec", "resolving hardwareSpec", err)
		}
		if err != nil {
			return ctrl.Result{}, errors.Wrap(err, "cannot update hardwareSpec")
		}
		if requeue {
			return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
		}
		err = r.createOrUpdateSatellitePod(ctx, req, &sat)
		if err != nil {
			_ = r.updateStatusCondition(ctx, &sat, "podCreation", "pod", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SatReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&yassv1.Sat{}).
		Named("sat").
		Complete(r)
}

func (r *SatReconciler) removeSatellite(ctx context.Context, req ctrl.Request) error {
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

func (r *SatReconciler) createOrUpdateSatellitePod(ctx context.Context, req ctrl.Request, sat *yassv1.Sat) error {
	podName := createPodName(req)
	pod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: podName}, pod)
	if apierrors.IsNotFound(err) {
		experimentName := sat.Labels[controller.LabelExperiment]
		// create Pod
		enginePorts := []v1.ContainerPort{}
		for port := 3000; port <= 3020; port++ {
			enginePorts = append(enginePorts, v1.ContainerPort{ContainerPort: int32(port)})
		}
		enginePorts = append(enginePorts, v1.ContainerPort{ContainerPort: int32(8080)})
		agentParameters, err := sat.Spec.Agent.ParametersAsMap("agent")
		if err != nil {
			return err
		}
		engineParameters, err := sat.Spec.Engine.ParametersAsMap("engine")
		if err != nil {
			return err
		}
		engineResources := v1.ResourceRequirements{Limits: map[v1.ResourceName]resource.Quantity{}}
		if sat.Spec.HardwareSpec.CPU != nil && !sat.Spec.HardwareSpec.CPU.IsZero() {
			engineResources.Limits[v1.ResourceCPU] = *sat.Spec.HardwareSpec.CPU
		}
		if sat.Spec.HardwareSpec.Memory != nil && !sat.Spec.HardwareSpec.Memory.IsZero() {
			engineResources.Limits[v1.ResourceMemory] = *sat.Spec.HardwareSpec.Memory
		}
		var containers []v1.Container
		containerSpecs := []containerSpec{
			{
				name:      "agent",
				image:     sat.Spec.Agent.Image,
				resources: v1.ResourceRequirements{},
				extraEnv:  agentParameters,
				ports:     nil,
			},
			{
				name:      "engine",
				image:     sat.Spec.Engine.Image,
				resources: engineResources,
				extraEnv:  engineParameters,
				ports:     enginePorts,
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
			container, err := r.createSatelliteContainer(sat, cs)
			if err != nil {
				return errors.Wrap(err, fmt.Sprintf("cannot create container %s", cs.name))
			}
			if container == nil {
				return fmt.Errorf("cannot create container %s, nil returned without error", cs.name)
			}
			containers = append(containers, *container)
		}

		pod = &v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: sat.Namespace,
				Labels: map[string]string{
					controller.LabelSatellite:  sat.Name,
					controller.LabelExperiment: experimentName,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(sat, v1.SchemeGroupVersion.WithKind("Sat")),
				},
			},
			Spec: v1.PodSpec{
				Volumes: []v1.Volume{
					{Name: sharedVolumeName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}},
					{Name: experimentName, VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{
						SizeLimit: sat.Spec.HardwareSpec.DiskSpace,
					}}},
				},
				Containers:         containers,
				ServiceAccountName: "",
			},
		}

		err = r.Create(ctx, pod)
		_ = r.updateStatusCondition(ctx, sat, "PodCreation", "creation", err)
		if err != nil {
			return err
		}
	}
	return nil
}

func createPodName(req ctrl.Request) string {
	return fmt.Sprintf("%s-pod", req.Name)
}

func (r *SatReconciler) createSatelliteContainer(sat *yassv1.Sat, cs containerSpec) (*v1.Container, error) {
	experimentName := sat.Labels[controller.LabelExperiment]
	envVars := []v1.EnvVar{
		{Name: controller.LabelSatellite, Value: sat.Name},
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
				MountPath: "/data",
			},
		},
		ImagePullPolicy: "Always",
	}
	return &container, nil
}

func (r *SatReconciler) updateHardwareSpec(ctx context.Context, sat *yassv1.Sat) (bool, error) {
	if sat.Spec.HardwareSpec == nil && sat.Spec.HardwareDefinitionRef != "" {
		hardwareDef := &yassv1.HardwareDefinition{}
		err := r.Get(ctx, types.NamespacedName{Name: sat.Spec.HardwareDefinitionRef}, hardwareDef)
		if apierrors.IsNotFound(err) {
			return false, fmt.Errorf("HardwareDefiniotn '%s' not found", sat.Spec.HardwareDefinitionRef)
		}
		if err != nil {
			return false, errors.Wrap(err, "cannot fetch HardwareDefinition")
		}
		sat.Spec.HardwareSpec = &hardwareDef.Spec
		err = r.Update(ctx, sat)
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func (r *SatReconciler) updateStatusCondition(ctx context.Context, sat *yassv1.Sat, ctype string, reason string, cause error) error {
	if reason == "" || cause == nil {
		reason = "ok"
	}
	var condition *metav1.Condition
	found := false
	for _, c := range sat.Status.Conditions {
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
		sat.Status.Conditions = append(sat.Status.Conditions, *condition)
	}
	err := r.Status().Update(ctx, sat)
	if err != nil {
		logf.FromContext(ctx).Error(err, fmt.Sprintf("cannot update sat %s status", sat.Name))
	}
	return cause
}
