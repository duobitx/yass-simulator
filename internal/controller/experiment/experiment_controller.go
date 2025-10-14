/*
 */

package experiment

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	yassv1 "github.com/ESA-PhiLab/yass-operator/api/v1"
	"github.com/ESA-PhiLab/yass-operator/internal/config"
	"github.com/ESA-PhiLab/yass-operator/internal/controller"
	"github.com/m-szalik/goutils"
	"github.com/m-szalik/goutils/collections"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	Configuration *config.Configuration
	recorder      record.EventRecorder
	Scheme        *runtime.Scheme
}

// + kubebuilder:rbac:groups=int.esa.yass,resources=experiments,verbs=get;list;watch;create;update;patch;delete
// + kubebuilder:rbac:groups=int.esa.yass,resources=experiments/status,verbs=get;update;patch
// + kubebuilder:rbac:groups=int.esa.yass,resources=experiments/finalizers,verbs=update
// + kubebuilder:rbac:groups=int.esa.yass,resources=fsnodes,verbs=get;list;watch;delete;create;update
// + kubebuilder:rbac:groups=int.esa.yass,resources=fsnodes/status,verbs=get;
// + kubebuilder:rbac:groups=int.esa.yass,resources=experimentdefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=*,resources=*,verbs=*
// TODO limit permissions

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *ExperimentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info(fmt.Sprintf("req %+v", req))

	var experiment yassv1.Experiment
	err := r.Get(ctx, req.NamespacedName, &experiment)
	if err != nil {
		if apierrors.IsNotFound(err) { // Resource experiment was deleted
			logger.Info("Experiment deleted", "name", req.NamespacedName)
			err = r.deleteExperimentObjects(ctx, req.NamespacedName.Namespace, req.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	} else {
		if experiment.Status.ExperimentState == "" {
			experiment.Status.ExperimentState = yassv1.ExperimentStateInit
		}
		err = r.createOrUpdateExperiment(ctx, req, &experiment)
		upErr := r.Status().Update(ctx, &experiment)
		if upErr != nil {
			logger.Error(upErr, "cannot update experiment status")
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		if experiment.Status.ExperimentState == yassv1.ExperimentStateInit {
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExperimentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("experiment-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&yassv1.Experiment{}).
		Owns(&yassv1.FsNode{}).
		Named("experiment-controller").
		Complete(r)
}

func (r *ExperimentReconciler) createOrUpdateExperiment(ctx context.Context, req ctrl.Request, experiment *yassv1.Experiment) error {
	exDef := yassv1.ExperimentDefinition{}
	if experiment.Spec.ExperimentDefRef == "" {
		r.updateStatusConditionForExperimentObject(experiment, "experiment-definition", &exDef, errors.New("experimentDefRef is empty"))
	} else {
		err := r.Get(ctx, types.NamespacedName{Name: experiment.Spec.ExperimentDefRef}, &exDef)
		r.updateStatusConditionForExperimentObject(experiment, "experiment-definition", &exDef, err)
	}

	layoutDef := yassv1.Layout{}
	if experiment.Spec.LayoutDefRef == "" {
		r.updateStatusConditionForExperimentObject(experiment, "layout", &exDef, errors.New("layoutDefRef is empty"))
	} else {
		err := r.Get(ctx, types.NamespacedName{Name: experiment.Spec.LayoutDefRef}, &layoutDef)
		r.updateStatusConditionForExperimentObject(experiment, "layout", &layoutDef, err)
	}

	componentDefinitions := []struct {
		fName    string
		compName string
		objSrc   client.Object
		mod      func(object client.Object)
	}{
		{"yaas-serviceaccount.yaml", "yass-sa", &v1.ServiceAccount{}, nil},
		{"messaging-statefulSet.yaml", "messaging", &appsv1.StatefulSet{}, modAddExperimentAnnotation(experiment.Name)},
		{"messaging-service.yaml", "messaging", &v1.Service{}, nil},
		{"experiment-executor-statefulSet.yaml", "experiment-executor", &appsv1.StatefulSet{}, modAddExperimentAnnotation(experiment.Name)},
		{"experiment-executor-service.yaml", "experiment-executor", &v1.Service{}, nil},
		{"yass-sa-role-binding.yaml", "yass-sa-role-binding", &rbacv1.ClusterRoleBinding{}, func(object client.Object) {
			rbacClusterRoleBinding := object.(*rbacv1.ClusterRoleBinding)
			rbacClusterRoleBinding.Subjects[0].Namespace = experiment.Namespace
		}},
	}
	joinErrHelper := &goutils.JoinErrorHelper{}
	for _, cDef := range componentDefinitions {
		objCopy := cDef.objSrc.DeepCopyObject()
		obj := objCopy.(client.Object)
		objErr := r.createExperimentComponentIfRequired(ctx, req.Namespace, experiment, cDef.fName, cDef.compName, obj, cDef.mod)
		if objErr != nil {
			joinErrHelper.Append(errors.Wrap(objErr, fmt.Sprintf("error creating experiment component %s/%s for %s from template %s", cDef.objSrc.GetObjectKind().GroupVersionKind(), cDef.compName, experiment.Name, cDef.fName)))
		}
	}
	err := joinErrHelper.AsError()
	if err != nil {
		return err
	}

	joinErrHelper = &goutils.JoinErrorHelper{}
	if layoutDef.Spec != nil {
		for _, satItem := range layoutDef.Spec {
			err = r.createFsNodeResource(ctx, req.NamespacedName.Namespace, experiment, &exDef, &satItem)
			joinErrHelper.Append(err)
		}
	}
	if err := joinErrHelper.AsError(); err != nil {
		return err
	}

	var failedComponents []string
	ready := collections.AllMatch(experiment.Status.Conditions, func(element *metav1.Condition) bool {
		if element.Status == metav1.ConditionFalse {
			failedComponents = append(failedComponents, element.Type)
		}
		return element.Status == metav1.ConditionTrue
	})
	if experiment.Status.ExperimentState == yassv1.ExperimentStateInit && ready {
		experiment.Status.ExperimentState = yassv1.ExperimentStateReady
		err = r.httpExperimentExecutor("start", nil, experiment)
		if err != nil {
			return errors.Wrap(err, "cannot start experiment")
		}
	}
	if !ready && (experiment.Status.ExperimentState == yassv1.ExperimentStateReady || experiment.Status.ExperimentState == yassv1.ExperimentStateOngoing) {
		experiment.Status.ExperimentState = yassv1.ExperimentStateErrored
		message := fmt.Sprintf("one or more components failed %s", strings.Join(failedComponents, ","))
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "componentFailed", message)
		err = r.httpExperimentExecutor("error-report", []byte(message), experiment)
		if err != nil {
			return errors.Wrap(err, "cannot start experiment")
		}
	}
	return nil
}

func (r *ExperimentReconciler) deleteExperimentObjects(ctx context.Context, namespace, experimentName string) error {
	logger := logf.FromContext(ctx)
	gvks := []client.ObjectList{
		&yassv1.FsNodeList{}, &v1.PodList{}, &v1.ServiceList{}, &v1.ConfigMapList{}, &v1.ServiceAccountList{},
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

func (r *ExperimentReconciler) createExperimentComponentIfRequired(ctx context.Context, namespace string, experiment *yassv1.Experiment, fName string, objName string, obj client.Object, modifier func(o client.Object)) (exitErr error) {
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: objName}, obj)
	defer func() {
		r.updateStatusConditionForExperimentObject(experiment, objName, obj, exitErr)
	}()
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
	return err
}

func (r *ExperimentReconciler) updateStatusConditionForExperimentObject(exp *yassv1.Experiment, compName string, obj client.Object, extra error) {
	ctype := fmt.Sprintf("%s-%s", compName, obj.GetObjectKind().GroupVersionKind().Kind)
	var condition *metav1.Condition
	found := false
	for _, c := range exp.Status.Conditions {
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
	newStatus := condition.Status
	newReason := condition.Reason
	newMessage := ""
	if extra != nil {
		newStatus = metav1.StatusFailure
		if apierrors.IsNotFound(extra) {
			newReason = "objectNotFound"
		} else {
			newReason = "error"
			newMessage = extra.Error()
		}
	} else {
		ready := false
		newReason = "notReady"
		switch x := obj.(type) {
		case *v1.Pod:
			ready = collections.AllMatch(x.Status.Conditions, func(element v1.PodCondition) bool {
				return element.Status == v1.ConditionTrue
			})
		case *appsv1.StatefulSet:
			ready = x.Status.AvailableReplicas > 0
			newReason = goutils.BoolToStr(ready, fmt.Sprintf("Replicas_%d", x.Status.AvailableReplicas), "notReadyAtLeastOneReplicaIsRequired")
		case *appsv1.Deployment:
			ready = x.Status.AvailableReplicas > 0
			newReason = goutils.BoolToStr(ready, fmt.Sprintf("Replicas_%d", x.Status.AvailableReplicas), "notReadyAtLeastOneReplicaIsRequired")
		case *yassv1.FsNode:
			ready = x.Status.Ready
		default:
			ready = true
		}
		if ready {
			newReason = "ok"
		}
		newStatus = goutils.BoolTo(ready, metav1.ConditionTrue, metav1.ConditionFalse)
	}

	if condition.Status != newStatus || condition.Reason != newReason || condition.Message != newMessage {
		condition.LastTransitionTime = metav1.Time{Time: time.Now()}
		condition.Reason = newReason
		condition.Status = newStatus
		condition.Message = newMessage
	}
	if !found {
		exp.Status.Conditions = append(exp.Status.Conditions, condition)
	}
}

func (r *ExperimentReconciler) createFsNodeResource(ctx context.Context, namespace string, experiment *yassv1.Experiment, expDef *yassv1.ExperimentDefinition, layoutItem *yassv1.LayoutSatSpec) (exitErr error) {
	fsNode := &yassv1.FsNode{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: layoutItem.FsNodeName}, fsNode)
	defer func() {
		r.updateStatusConditionForExperimentObject(experiment, layoutItem.FsNodeName, fsNode, exitErr)
	}()
	if apierrors.IsNotFound(err) {
		var behaviour *yassv1.Behaviour
		for _, sb := range expDef.Spec.Behaviours {
			if sb.FsNodeName == layoutItem.FsNodeName {
				behaviour = &sb
				break
			}
		}
		if behaviour == nil {
			return fmt.Errorf("cannot find fsNode item in experimentDefinition for '%s'", layoutItem.FsNodeName)
		}
		fsNode = &yassv1.FsNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      layoutItem.FsNodeName,
				Namespace: namespace,
				Labels: map[string]string{
					controller.LabelExperiment: experiment.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(experiment, v1.SchemeGroupVersion.WithKind(experimentKind)),
				},
			},
			Spec: yassv1.FsNodeSpec{
				EmbeddedHardware: layoutItem.EmbeddedHardware,
				EmbeddedPosition: layoutItem.EmbeddedPosition,
				Engine:           experiment.Spec.Engine,
				Agent:            behaviour.Agent,
			},
		}
		err = r.Create(ctx, fsNode)
		return err
	}
	return nil
}

func (r *ExperimentReconciler) httpExperimentExecutor(endpoint string, body []byte, experiment *yassv1.Experiment) error {
	response, err := http.Post(fmt.Sprintf("http://experiment-executor.%s.svc.cluster.local:8080/%s", experiment.Namespace, endpoint), goutils.BoolToStr(body != nil, "application/json", ""), bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "experimentStarted", "unable to start experiment - %s", string(body))
	} else {
		body, _ := io.ReadAll(response.Body)
		r.recorder.Eventf(experiment, v1.EventTypeNormal, "experimentStarted", "experiment started - %s", string(body))
		experiment.Status.ExperimentState = yassv1.ExperimentStateOngoing
	}
	return nil
}
