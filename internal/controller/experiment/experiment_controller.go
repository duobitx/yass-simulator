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

package experiment

import (
	"context"
	errors2 "errors"
	"fmt"
	"strings"
	"time"

	yassv1 "github.com/ESA-PhiLab/yass-experiment-operator/api/v1"
	"github.com/ESA-PhiLab/yass-experiment-operator/internal/controller"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const componentSelectorLabel = "component"

// ExperimentReconciler reconciles an Experiment object
type ExperimentReconciler struct {
	client.Client
	recorder record.EventRecorder
	Scheme   *runtime.Scheme
}

// +kubebuilder:rbac:groups=int.esa.yass,resources=experiments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=int.esa.yass,resources=experiments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=int.esa.yass,resources=experiments/finalizers,verbs=update
// +kubebuilder:rbac:groups=int.esa.yass,resources=sats,verbs=get;list;watch;delete
// +kubebuilder:rbac:groups=int.esa.yass,resources=experimentdefinitions,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Guestbook object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *ExperimentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info(fmt.Sprintf("req %+v", req))

	// Fetch the resource
	var experiment yassv1.Experiment
	err := r.Get(ctx, req.NamespacedName, &experiment)

	if err != nil {
		if apierrors.IsNotFound(err) {
			// 🔴 Resource was deleted
			logger.Info("Experiment deleted", "name", req.NamespacedName)
			var errs []error
			err = r.deleteSatellites(ctx, req.NamespacedName.Namespace, req.Name)
			if err != nil {
				errs = append(errs, err)
			}
			err = r.deleteExperimentComponents(ctx, req.NamespacedName.Namespace, req.Name)
			if err != nil {
				errs = append(errs, err)
			}
			if len(errs) > 0 {
				return ctrl.Result{}, errors2.Join(errs...)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	} else {
		err = r.createOrUpdateExperiment(ctx, req, &experiment)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExperimentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("my-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&yassv1.Experiment{}).
		Named("experiment").
		Complete(r)
}

func (r *ExperimentReconciler) createOrUpdateExperiment(ctx context.Context, req ctrl.Request, experiment *yassv1.Experiment) error {
	experimentComponentsToCreate := experimentComponents()
	for _, expComp := range experimentComponentsToCreate {
		err := r.shouldHaveComponent(ctx, req.Namespace, &expComp, experiment)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("error creating component pod %s for experiment %s", expComp.name, experiment.Name))
		}
	}
	if experiment.Spec.ExperimentDefRef == "" {
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "experimentDefRef not defined", ".spec.experimentDefRef is empty")
		return fmt.Errorf("experiment.spec.experimentDefRef must not be empty")
	}
	expDef := &yassv1.ExperimentDefinition{}
	if err := r.Get(ctx, types.NamespacedName{Name: experiment.Spec.ExperimentDefRef}, expDef); err != nil {
		if apierrors.IsNotFound(err) {
			r.recorder.Eventf(experiment, v1.EventTypeWarning, "experimentDefRef not found", "experimentDefinition %s not found", experiment.Spec.ExperimentDefRef)
			logf.FromContext(ctx).Info("ExperimentDefinition not found", "name", experiment.Spec.ExperimentDefRef)
			// Gracefully ignore to allow controller to requeue on future changes
			return nil
		}
		return err
	}
	r.recorder.Eventf(experiment, v1.EventTypeNormal, "experimentDefRef", "experimentDefinition %s found", experiment.Spec.ExperimentDefRef)
	if experiment.Spec.LayoutDefRef == "" {
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "layoutRef not defined", ".spec.layoutDefRef is empty")
		return fmt.Errorf("experiment.spec.layoutDefRef must not be empty")
	}
	layoutDef := &yassv1.Layout{}
	if err := r.Get(ctx, types.NamespacedName{Name: experiment.Spec.LayoutDefRef}, layoutDef); err != nil {
		if apierrors.IsNotFound(err) {
			logf.FromContext(ctx).Info("Layout not found", "name", experiment.Spec.LayoutDefRef)
			r.recorder.Eventf(experiment, v1.EventTypeWarning, "layoutDefRef not found", "layout %s not found", experiment.Spec.LayoutDefRef)
			// Gracefully ignore to allow controller to requeue on future changes
			return nil
		}
		return err
	}
	r.recorder.Eventf(experiment, v1.EventTypeNormal, "layoutDefRef found", "layout %s found", experiment.Spec.LayoutDefRef)
	for _, satItem := range layoutDef.Spec {
		err := r.createSatelliteResource(ctx, req.NamespacedName.Namespace, experiment, expDef, &satItem)
		_ = r.updateStatusCondition(ctx, experiment, fmt.Sprintf("sat_creation_%s", satItem.SatName), "", err)
		if err != nil {
			return err
		}
	}
	if experiment.Spec.Started {
		err := r.startExperiment(ctx, experiment)
		if err != nil {
			return errors.Wrap(err, "unable to start experiment")
		}
	}
	return nil
}

type expComponent struct {
	name         string
	podSpec      func(podSpec *v1.PodSpec)
	servicePorts []int
}

func experimentComponents() []expComponent {
	return []expComponent{
		{
			name:         "messaging",
			servicePorts: []int{1883},
			podSpec: func(podSpec *v1.PodSpec) {
				podSpec.Containers = []v1.Container{
					{
						Name:            "main",
						Image:           "ghcr.io/esa-philab/yass/eclipse-mosquitto:latest",
						Ports:           []v1.ContainerPort{{ContainerPort: 1883}},
						ImagePullPolicy: "Always",
					},
				}
			},
		},
		{
			name:         "messaging-gw",
			servicePorts: []int{8080},
			podSpec: func(podSpec *v1.PodSpec) {
				probe := &v1.Probe{
					ProbeHandler: v1.ProbeHandler{
						HTTPGet: &v1.HTTPGetAction{
							Path: "/health-check",
							Port: intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
						},
					},
					InitialDelaySeconds: 3,
					TimeoutSeconds:      3,
					PeriodSeconds:       20,
				}
				podSpec.Containers = []v1.Container{
					{
						Name:            "main",
						Image:           "ghcr.io/esa-philab/yass/gateway:latest",
						Ports:           []v1.ContainerPort{{ContainerPort: 8080}},
						Env:             []v1.EnvVar{{Name: "HANDLERS", Value: "mqtt:messaging:1883:::"}},
						ImagePullPolicy: "Always",
						LivenessProbe:   probe,
						ReadinessProbe:  probe,
					},
				}
			},
		},
	}
}

func (r *ExperimentReconciler) deleteSatellites(ctx context.Context, namespace, experimentName string) error {
	logger := logf.FromContext(ctx)
	satList := &yassv1.SatList{}
	if err := r.List(ctx, satList,
		client.InNamespace(namespace),
		client.MatchingLabels{controller.LabelExperiment: experimentName},
	); err != nil {
		return err
	}
	for _, sat := range satList.Items {
		if err := r.Delete(ctx, &sat); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		logger.Info("Deleted Sat for Experiment", "experiment", experimentName, "sat", sat.Name, "namespace", sat.Namespace)
	}
	return nil
}

func (r *ExperimentReconciler) deleteExperimentComponents(ctx context.Context, namespace string, experimentName string) error {
	logger := logf.FromContext(ctx)
	podList := &v1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels{controller.LabelExperiment: experimentName},
	); err != nil {
		return err
	}
	for _, pod := range podList.Items {
		err := r.Delete(ctx, &pod)
		if err != nil {
			logger.Error(err, fmt.Sprintf("error deleting pod %s.%s", namespace, pod.Name))
		} else {
			logger.Info(fmt.Sprintf("Pod %s.%s deleted", namespace, pod.Name))
		}
	}

	serviceList := &v1.ServiceList{}
	if err := r.List(ctx, serviceList,
		client.InNamespace(namespace),
		client.MatchingLabels{controller.LabelExperiment: experimentName},
	); err != nil {
		return err
	}
	for _, svc := range serviceList.Items {
		err := r.Delete(ctx, &svc)
		if err != nil {
			logger.Error(err, fmt.Sprintf("error deleting service %s.%s", namespace, svc.Name))
		} else {
			logger.Info(fmt.Sprintf("Service %s.%s deleted", namespace, svc.Name))
		}
	}
	return nil
}

func (r *ExperimentReconciler) shouldHaveComponent(ctx context.Context, namespace string, componentSpec *expComponent, experiment *yassv1.Experiment) error {
	pod := &v1.Pod{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: componentSpec.name}, pod)
	if err == nil {
		// pod exists
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	restartPolicy := v1.RestartPolicyOnFailure
	pod = &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentSpec.name,
			Namespace: namespace,
			Labels: map[string]string{
				controller.LabelExperiment: experiment.Name,
				componentSelectorLabel:     componentSpec.name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(experiment, v1.SchemeGroupVersion.WithKind("Experiment")),
			},
		},
		Spec: v1.PodSpec{
			ServiceAccountName: "",
			RestartPolicy:      restartPolicy,
		},
	}
	componentSpec.podSpec(&pod.Spec)
	err = r.Create(ctx, pod)
	_ = r.updateStatusCondition(ctx, experiment, fmt.Sprintf("%s-pod", componentSpec.name), "creating", err)
	err2 := r.updateStatusCondition(ctx, experiment, fmt.Sprintf("%s-pod", componentSpec.name), pod.Status.Message, nil)
	if err2 != nil {
		logf.FromContext(ctx).Error(err, "cannot update status.condition for pod")
	}
	if err != nil {
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "ExperimentComponentCreation", "Component pod %s create error: %s", componentSpec.name, err.Error())
		return err
	}
	r.recorder.Eventf(experiment, v1.EventTypeNormal, "ExperimentComponentCreation", "Component pod %s created", componentSpec.name)
	if len(componentSpec.servicePorts) > 0 {
		ports := []v1.ServicePort{}
		for _, port := range componentSpec.servicePorts {
			ports = append(ports, v1.ServicePort{
				Port: int32(port),
				TargetPort: intstr.IntOrString{
					Type:   intstr.Int,
					IntVal: int32(port),
				},
			})
		}
		service := v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      componentSpec.name,
				Namespace: pod.Namespace,
				Labels:    pod.Labels,
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(experiment, v1.SchemeGroupVersion.WithKind("Experiment")),
				},
			},
			Spec: v1.ServiceSpec{
				Ports: ports,
				Selector: map[string]string{
					componentSelectorLabel: componentSpec.name,
				},
			},
		}
		instance := &v1.Service{}
		err = r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: service.Name}, instance)
		if err == nil {
			// service exists
			return nil
		}
		if !apierrors.IsNotFound(err) {
			return err
		}
		err = r.Create(ctx, &service)
		err2 := r.updateStatusCondition(ctx, experiment, fmt.Sprintf("%s-service", componentSpec.name), "", err)
		if err2 != nil {
			logf.FromContext(ctx).Error(err, "cannot update status.condition for service")
		}
		if err != nil {
			r.recorder.Eventf(experiment, v1.EventTypeWarning, "ExperimentComponentCreation", "Component service %s create error: %s", componentSpec.name, err.Error())
			return err
		}
		r.recorder.Eventf(experiment, v1.EventTypeNormal, "ExperimentComponentCreation", "Component service %s created", componentSpec.name)
		return nil
	}
	return nil
}

func (r *ExperimentReconciler) updateStatusCondition(ctx context.Context, exp *yassv1.Experiment, ctype string, reason string, cause error) error {
	if reason == "" || cause == nil {
		reason = "ok"
	}
	var condition *metav1.Condition
	found := false
	for _, c := range exp.Status.Conditions {
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
		exp.Status.Conditions = append(exp.Status.Conditions, *condition)
	}
	err := r.Status().Update(ctx, exp)
	if err != nil {
		logf.FromContext(ctx).Error(err, fmt.Sprintf("cannot update experiment %s status", exp.Name))
	}
	return cause
}

func (r *ExperimentReconciler) createSatelliteResource(ctx context.Context, namespace string, experiment *yassv1.Experiment, expDef *yassv1.ExperimentDefinition, layoutItem *yassv1.LayoutSatSpec) error {
	sat := &yassv1.Sat{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: layoutItem.SatName}, sat)
	if apierrors.IsNotFound(err) {
		var satBehaviour *yassv1.SatBehaviour
		for _, sb := range expDef.Spec.SatBehaviours {
			if sb.SatName == layoutItem.SatName {
				satBehaviour = &sb
				break
			}
		}
		if satBehaviour == nil {
			return fmt.Errorf("cannot find sat item in experimentDefinition for '%s'", layoutItem.SatName)
		}
		sat = &yassv1.Sat{
			ObjectMeta: metav1.ObjectMeta{
				Name:      layoutItem.SatName,
				Namespace: namespace,
				Labels: map[string]string{
					controller.LabelExperiment: experiment.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(experiment, v1.SchemeGroupVersion.WithKind("Experiment")),
				},
			},
			Spec: yassv1.SatSpec{
				HardwareSpec: &layoutItem.HardwareSpec,
				Orbit:        layoutItem.Orbit,
				Rotation:     layoutItem.Rotation,
				Engine:       experiment.Spec.Engine,
				Agent:        satBehaviour.Agent,
			},
		}
		err = r.Create(ctx, sat)
		_ = r.updateStatusCondition(ctx, experiment, fmt.Sprintf("Sat-%s", layoutItem.SatName), "creating", err)
		if err != nil {
			r.recorder.Eventf(experiment, v1.EventTypeWarning, "sat creation", "sat %s error :: %s", sat.Name, err)
			return err
		}
		r.recorder.Eventf(experiment, v1.EventTypeNormal, "sat creation", "sat %s :: online", sat.Name)
	}
	return nil
}

func (r *ExperimentReconciler) startExperiment(ctx context.Context, experiment *yassv1.Experiment) error {
	// TODO call experiment executor
	experiment.Status.Started = &metav1.Time{Time: time.Now()}
	err := r.Status().Update(ctx, experiment)
	if err != nil {
		logf.FromContext(ctx).Error(err, fmt.Sprintf("cannot update experiment %s status (started)", experiment.Name))
	}
	return nil
}
