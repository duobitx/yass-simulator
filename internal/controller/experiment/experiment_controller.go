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
	"fmt"
	"os"
	"strings"
	"time"

	yassv1 "github.com/ESA-PhiLab/yass-experiment-operator/api/v1"
	"github.com/ESA-PhiLab/yass-experiment-operator/internal/controller"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	experimentKind = "Experiment"
)

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
			// Resource experiment was deleted
			logger.Info("Experiment deleted", "name", req.NamespacedName)
			err = r.deleteExperimentObjects(ctx, req.NamespacedName.Namespace, req.Name)
			return ctrl.Result{}, err
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
	r.recorder = mgr.GetEventRecorderFor("experiment-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&yassv1.Experiment{}).
		Named("experiment").
		Complete(r)
}

func (r *ExperimentReconciler) createOrUpdateExperiment(ctx context.Context, req ctrl.Request, experiment *yassv1.Experiment) error {
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

	componentDefs := []struct {
		fName    string
		compName string
		objSrc   client.Object
		mod      func(object client.Object)
	}{
		{"yaas-serviceaccount.yaml", "yass-sa", &v1.ServiceAccount{}, nil},
		{"messaging-statefulSet.yaml", "messaging", &appsv1.StatefulSet{}, nil},
		{"messaging-service.yaml", "messaging", &v1.Service{}, nil},
		//{"executor-statefulSet.yaml", "executor", &appsv1.StatefulSet{}, nil},
		//{"executor-service.yaml", "executor", &v1.Service{}, nil},
	}
	joinErrHelper := &goutils.JoinErrorHelper{}
	for _, cDef := range componentDefs {
		objCopy := cDef.objSrc.DeepCopyObject()
		obj := objCopy.(client.Object)
		objErr := r.createExperimentObjectIfRequired(ctx, req.Namespace, experiment, cDef.fName, cDef.compName, obj, cDef.mod)
		_ = r.updateStatusCondition(ctx, experiment, cDef.compName, "creation", objErr)
		if objErr != nil {
			joinErrHelper.Append(errors.Wrap(objErr, fmt.Sprintf("error creating experiment component %s/%s for %s from template %s", cDef.objSrc.GetObjectKind().GroupVersionKind(), cDef.compName, experiment.Name, cDef.fName)))
		}
	}
	err := joinErrHelper.AsError()
	if err != nil {
		return err
	}

	for _, satItem := range layoutDef.Spec {
		err := r.createSatelliteResource(ctx, req.NamespacedName.Namespace, experiment, expDef, &satItem)
		_ = r.updateStatusCondition(ctx, experiment, fmt.Sprintf("sat_creation_%s", satItem.SatName), "", err)
		if err != nil {
			return err
		}
	}
	if experiment.Spec.Start {
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

func (r *ExperimentReconciler) deleteExperimentObjects(ctx context.Context, namespace, experimentName string) error {
	logger := logf.FromContext(ctx)
	gvks := []client.ObjectList{
		&yassv1.SatList{}, &v1.PodList{}, &v1.ServiceList{}, &v1.ConfigMapList{}, &v1.ServiceAccountList{},
		&appsv1.DeploymentList{}, &appsv1.StatefulSetList{},
	}
	for _, objList := range gvks {
		if err := r.List(ctx, objList,
			client.InNamespace(namespace),
			client.MatchingLabels(map[string]string{controller.LabelExperiment: experimentName}),
		); err != nil {
			return err
		}
		accessor := meta.NewAccessor()
		items, err := meta.ExtractList(objList)
		if err != nil {
			return err
		}
		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			ownedByExperiment := false
			for _, ownerRef := range obj.GetOwnerReferences() {
				if ownerRef.Kind == experimentKind && ownerRef.Name == experimentName {
					ownedByExperiment = true
					break
				}
			}
			if ownedByExperiment {
				if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
				name, _ := accessor.Name(obj)
				ns, _ := accessor.Namespace(obj)
				logger.Info("Deleted resource", "kind", item.GetObjectKind().GroupVersionKind().Kind, "name", name, "namespace", ns)
			}
		}
	}
	return nil
}

func (r *ExperimentReconciler) createExperimentObjectIfRequired(ctx context.Context, namespace string, experiment *yassv1.Experiment, fName string, objName string, obj client.Object, modifier func(o client.Object)) error {
	logger := logf.FromContext(ctx)
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: objName}, obj)
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	fn := fmt.Sprintf("obj-templates/%s", fName)
	buff, err := os.ReadFile(fn)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("cannot read file %s", fn))
	}
	err = yaml.Unmarshal(buff, obj)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("cannot unmarshall file %s", fn))
	}
	obj.SetNamespace(namespace)
	obj.SetName(objName)
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[controller.LabelExperiment] = experiment.Name
	obj.SetLabels(labels)
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	labels["component-source"] = fName
	obj.SetAnnotations(annotations)
	obj.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(experiment, v1.SchemeGroupVersion.WithKind(experiment.Kind))})
	if modifier != nil {
		modifier(obj)
	}
	err = r.Create(ctx, obj)
	if err != nil {
		r.recorder.Eventf(experiment, v1.EventTypeWarning, objName, fmt.Sprintf("creation error: %s", err.Error()))
		return err
	}
	r.recorder.Eventf(experiment, v1.EventTypeNormal, objName, "component created")
	logger.Info(fmt.Sprintf("object %s of type %T created", objName, obj))
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
	ready := false
	if len(exp.Status.Conditions) > 0 {
		allOk := false
		for _, cond := range exp.Status.Conditions {
			if string(cond.Status) == string(v1.ConditionFalse) {
				allOk = false
				break
			}
		}
		ready = allOk
	}
	exp.Status.Ready = ready
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
					*metav1.NewControllerRef(experiment, v1.SchemeGroupVersion.WithKind(experimentKind)),
				},
			},
			Spec: yassv1.SatSpec{
				EmbeddedHardware: layoutItem.EmbeddedHardware,
				EmbeddedPosition: layoutItem.EmbeddedPosition,
				Engine:           experiment.Spec.Engine,
				Agent:            satBehaviour.Agent,
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
