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
	"strconv"
	"strings"
	"text/template"
	"time"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"github.com/duobitx/yass-simulator/yass-operator/internal/config"
	"github.com/duobitx/yass-simulator/yass-operator/internal/controller"
	"github.com/m-szalik/goutils"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	experimentKind = "Experiment"

	// apiServiceGroupVersion is the group/version of the aggregated runtime API
	// (a kubernetes APIService) that fronts an experiment's runtime functions.
	// status.apiServerURL points at this experiment's base path under it.
	apiServiceGroupVersion = "runtime.esa.yass/v1"

	// MaxFsNodes is the maximum total FsNode count an Experiment Layout may
	// reference. Must stay in sync with the MAX_FSNODES cap in
	// internal-components/geo-calculator/V6/geo_calc.cc — exceeding it makes
	// geo_calc reject the input and the experiment cannot run.
	MaxFsNodes = 1024
)

// Reconciler reconciles an Experiment object
type Reconciler struct {
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

const (
	removeFsNodesFinalizer = "experiment-controller/cleanup-fsNodes"
)

type reconciliationStatus struct {
	statusUpdated bool
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (exitRet ctrl.Result, exitErr error) {
	logger := logf.FromContext(ctx)
	logger.Info(fmt.Sprintf("req %+v", req))

	var experiment yassv1.Experiment
	err := r.Get(ctx, req.NamespacedName, &experiment)
	if err != nil {
		if apierrors.IsNotFound(err) { // Resource not found - ignore
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	if !experiment.DeletionTimestamp.IsZero() {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(&experiment, removeFsNodesFinalizer) {
			// Run cleanup logic
			logger.Info("Experiment deleted", "name", req.NamespacedName)
			err = r.deleteExperimentObjects(ctx, req.Namespace, req.Name)
			if err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(&experiment, removeFsNodesFinalizer)
			if err := r.Update(ctx, &experiment); err != nil {
				return ctrl.Result{RequeueAfter: 1 * time.Second}, err
			}
		}
		return ctrl.Result{}, nil
	}
	// Skip reconciling into a namespace that is being deleted;
	if terminating, nsErr := controller.NamespaceTerminating(ctx, r.Client, experiment.Namespace); nsErr != nil {
		return ctrl.Result{}, nsErr
	} else if terminating {
		return ctrl.Result{}, nil
	}

	logger.Info(fmt.Sprintf("req %+v, experiment.status: %+v", req, experiment.Status))

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(&experiment, removeFsNodesFinalizer) {
		controllerutil.AddFinalizer(&experiment, removeFsNodesFinalizer)
		if experiment.Spec.SimulationStartTime == nil || experiment.Spec.SimulationStartTime.IsZero() {
			experiment.Status.ExperimentTime = metav1.Now()
		} else {
			experiment.Status.ExperimentTime = *experiment.Spec.SimulationStartTime
		}
		if err := r.Update(ctx, &experiment); err != nil {
			return ctrl.Result{}, err
		}
	}
	recon := &reconciliationStatus{}
	defer func() {
		if recon.statusUpdated {
			upErr := r.Status().Update(ctx, &experiment)
			if upErr != nil {
				logger.Error(upErr, "cannot update experiment status")
			}
		}
	}()
	if experiment.Status.ExperimentState == "" {
		r.updateExperimentState(recon, &experiment, yassv1.ExperimentStateInit)
	}
	if url := fmt.Sprintf("/apis/%s/namespaces/%s/experiments/%s", apiServiceGroupVersion, experiment.Namespace, experiment.Name); experiment.Status.ApiServerURL != url {
		experiment.Status.ApiServerURL = url
		recon.statusUpdated = true
	}
	// Once resources have been evicted (see Spec.EvictResourcesAfter) the
	// experiment is a kept-but-emptied shell: do not recreate its workloads or
	// FsNodes, leave the (terminal) status as-is.
	if resourcesEvicted(&experiment) {
		return ctrl.Result{}, nil
	}
	err = r.createOrUpdateExperiment(recon, ctx, req, &experiment)

	if err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}
	if err := r.evaluateExperimentOutcome(recon, ctx, &experiment); err != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}
	if experiment.Status.ExperimentState == yassv1.ExperimentStateInit ||
		experiment.Status.ExperimentState == yassv1.ExperimentStateInsufficientResources {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	// Spec.Start=true asks the operator to POST /start once the
	// experiment goes Ready. If state is still Ready after the
	// reconcile, the start attempt either DNS-raced (StartPending
	// event) or simply hasn't happened yet — either way requeue
	// quickly without involving exponential backoff.
	if experiment.Status.ExperimentState == yassv1.ExperimentStateReady && experiment.Spec.Start {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if requeueAfter, evErr := r.maybeEvictResources(ctx, &experiment); evErr != nil {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, evErr
	} else if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("experiment-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&yassv1.Experiment{}).
		Owns(&yassv1.FsNode{}).
		Watches(&v1.Pod{}, handler.EnqueueRequestsFromMapFunc(mapPodToExperiment)).
		Named("experiment-controller").
		Complete(r)
}

func mapPodToExperiment(_ context.Context, obj client.Object) []reconcile.Request {
	expName := obj.GetLabels()[controller.LabelExperiment]
	if expName == "" {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: obj.GetNamespace(), Name: expName}}}
}

func (r *Reconciler) createOrUpdateExperiment(recon *reconciliationStatus, ctx context.Context, req ctrl.Request, experiment *yassv1.Experiment) error {
	exDef := yassv1.ExperimentDefinition{}
	if experiment.Spec.ExperimentDefRef == "" {
		r.updateStatusConditionForExperimentObject(recon, experiment, "experiment-definition", "ExperimentDefinition", &exDef, errors.New("experimentDefRef is empty"))
	} else {
		err := r.Get(ctx, types.NamespacedName{Name: experiment.Spec.ExperimentDefRef}, &exDef)
		r.updateStatusConditionForExperimentObject(recon, experiment, "experiment-definition", "ExperimentDefinition", &exDef, err)
	}

	layoutDef := yassv1.Layout{}
	if experiment.Spec.LayoutDefRef == "" {
		r.updateStatusConditionForExperimentObject(recon, experiment, "layout", "Layout", &exDef, errors.New("layoutDefRef is empty"))
	} else {
		err := r.Get(ctx, types.NamespacedName{Name: experiment.Spec.LayoutDefRef}, &layoutDef)
		r.updateStatusConditionForExperimentObject(recon, experiment, "layout", "Layout", &layoutDef, err)
	}

	if n := len(layoutDef.Spec); n > MaxFsNodes {
		msg := fmt.Sprintf("layout %s references %d FsNodes; maximum supported is %d (geo-calculator MAXSAT). Reduce the layout to start the experiment.", experiment.Spec.LayoutDefRef, n, MaxFsNodes)
		r.updateStatusConditionForExperimentObject(recon, experiment, "layout-size", "LayoutSize", &layoutDef, errors.New(msg))
		if experiment.Status.ExperimentState != yassv1.ExperimentStateErrored {
			r.updateExperimentState(recon, experiment, yassv1.ExperimentStateErrored)
			r.recorder.Eventf(experiment, v1.EventTypeWarning, "LayoutTooLarge", msg)
		}
		return nil
	}

	componentDefinitions := []struct {
		fName    string
		compName string
		resName  string
		objSrc   client.Object
		mod      func(object client.Object)
	}{
		{"messaging-statefulSet.yaml", "messaging", "", &appsv1.StatefulSet{}, modAddExperimentAnnotation(experiment.Name)},
		{"messaging-service.yaml", "messaging", "", &v1.Service{}, nil},
		{"fsnode-broadcast-service.yaml", "fsnode-broadcast", "", &v1.Service{}, nil},
		{"experiment-executor-statefulSet.yaml", "experiment-executor", "", &appsv1.StatefulSet{}, modAddExperimentAnnotation(experiment.Name)},
		{"experiment-executor-service.yaml", "experiment-executor", "", &v1.Service{}, nil},
		{"events-webapp-deployment.yaml", "events-webapp", "", &appsv1.Deployment{}, modAddExperimentAnnotation(experiment.Name)},
		{"events-webapp-service.yaml", "events-webapp", "", &v1.Service{}, nil},
		{"metrics-bridge-deployment.yaml", "metrics-bridge", "", &appsv1.Deployment{}, modMetricsBridge(experiment, deliveryDeadlineFor(exDef.Spec.MaxDuration))},
		{"mqtt2prom-deployment.yaml", "mqtt2prom", "", &appsv1.Deployment{}, modAddExperimentAnnotation(experiment.Name)},
		{"mqtt2prom-service.yaml", "mqtt2prom", "", &v1.Service{}, nil},
		{"web-ui-deployment.yaml", "web-ui", "", &appsv1.Deployment{}, modAddExperimentAnnotation(experiment.Name)},
		{"web-ui-service.yaml", "web-ui", "", &v1.Service{}, modAddExperimentAnnotation(experiment.Name)},
		{"web-ui-ingress.yaml", "web-ui", experiment.Name, &netv1.Ingress{}, modAddExperimentAnnotation(experiment.Name)},
	}
	joinErrHelper := &goutils.JoinErrorHelper{}
	for _, cDef := range componentDefinitions {
		objCopy := cDef.objSrc.DeepCopyObject()
		obj := objCopy.(client.Object)
		objErr := r.createExperimentComponentIfRequired(recon, ctx, req.Namespace, experiment, cDef.fName, cDef.compName, cDef.resName, obj, cDef.mod)
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
			fsNode, err := r.createFsNodeResource(ctx, req.Namespace, experiment, &exDef, &satItem)
			name := fmt.Sprintf("fsNode-%s", satItem.FsNodeName)
			r.updateStatusConditionForExperimentObject(recon, experiment, name, name, fsNode, err)
			joinErrHelper.Append(err)
		}
	}
	if err := joinErrHelper.AsError(); err != nil {
		return err
	}

	var failedComponents []string
	ready := goutils.AllMatch(experiment.Status.Conditions, func(element *metav1.Condition) bool {
		if element.Status == metav1.ConditionFalse {
			failedComponents = append(failedComponents, element.Type)
		}
		return element.Status == metav1.ConditionTrue
	})
	// Surface a lack of cluster capacity instead of hanging silently in Init: while
	// still coming up, if any pod is unschedulable flag InsufficientResources; when
	// capacity returns, recover to Init so the normal Init->Ready->Ongoing path runs.
	if s := experiment.Status.ExperimentState; s == yassv1.ExperimentStateInit || s == yassv1.ExperimentStateInsufficientResources {
		if unsched, detail, perr := r.hasUnschedulablePods(ctx, req.Namespace, experiment.Name); perr == nil {
			if unsched && s != yassv1.ExperimentStateInsufficientResources {
				r.updateExperimentState(recon, experiment, yassv1.ExperimentStateInsufficientResources)
				r.recorder.Eventf(experiment, v1.EventTypeWarning, "InsufficientResources", "pod(s) unschedulable: %s", detail)
			} else if !unsched && s == yassv1.ExperimentStateInsufficientResources {
				r.updateExperimentState(recon, experiment, yassv1.ExperimentStateInit)
			}
		}
	}
	if experiment.Status.ExperimentState == yassv1.ExperimentStateInit && ready {
		r.updateExperimentState(recon, experiment, yassv1.ExperimentStateReady)
	}
	if !ready && (experiment.Status.ExperimentState == yassv1.ExperimentStateReady || experiment.Status.ExperimentState == yassv1.ExperimentStateOngoing) {
		r.updateExperimentState(recon, experiment, yassv1.ExperimentStateErrored)
		message := fmt.Sprintf("one or more components failed %s", strings.Join(failedComponents, ","))
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "componentFailed", message)
		err = r.httpExperimentExecutor(ctx, recon, "error-report", []byte(message), experiment)
		if err != nil {
			return errors.Wrap(err, "cannot start experiment")
		}
	}
	// If any experiment component restarts *after* the run reaches Ongoing
	if experiment.Status.ExperimentState == yassv1.ExperimentStateOngoing {
		total, details, rErr := r.experimentRestartTotal(ctx, req.Namespace)
		if rErr != nil {
			return errors.Wrap(rErr, "cannot read experiment pod restart counts")
		}
		baseline, ok := restartBaseline(experiment)
		if !ok {
			if experiment.Annotations == nil {
				experiment.Annotations = map[string]string{}
			}
			experiment.Annotations[restartBaselineAnnotation] = strconv.Itoa(int(total))
			if err := r.Update(ctx, experiment); err != nil {
				return errors.Wrap(err, "cannot record restart baseline")
			}
		} else if total > baseline {
			r.updateExperimentState(recon, experiment, yassv1.ExperimentStateErrored)
			msg := fmt.Sprintf("component(s) restarted after start (baseline %d, now %d): %s; simulation state lost, run cannot resume",
				baseline, total, strings.Join(details, ", "))
			r.recorder.Eventf(experiment, v1.EventTypeWarning, "ComponentRestarted", msg)
		}
	}
	if experiment.Status.ExperimentState == yassv1.ExperimentStateReady && experiment.Spec.Start {
		err = r.httpExperimentExecutor(ctx, recon, "start", nil, experiment)
		if err != nil {
			if errors.Is(err, errExecutorDNSPending) {
				// DNS race: the Service was just created and CoreDNS
				// hasn't picked it up yet. Treat as a normal pending
				// state; the Reconcile loop requeues quickly so we
				// retry within a few seconds without exponential
				// backoff.
				r.recorder.Eventf(experiment, v1.EventTypeNormal, "StartPending", "executor service not yet in DNS, will retry")
				return nil
			}
			r.recorder.Eventf(experiment, v1.EventTypeWarning, "StartSignalError", "error starting experiment - %s", err)
			return errors.Wrap(err, "cannot start experiment")
		}
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "StartSignalSent", "")
	}
	return nil
}

func (r *Reconciler) deleteExperimentObjects(ctx context.Context, namespace, experimentName string) error {
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

func (r *Reconciler) createExperimentComponentIfRequired(recon *reconciliationStatus, ctx context.Context, namespace string, experiment *yassv1.Experiment, fName string, objName string, resName string, obj client.Object, modifier func(o client.Object)) (exitErr error) {
	if resName == "" {
		resName = objName
	}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: resName}, obj)
	defer func() {
		r.updateStatusConditionForExperimentObject(recon, experiment, objName, kebabToCamel(objName), obj, exitErr)
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
	values := map[string]any{
		"templateFile":           fName,
		"experiment":             experiment,
		"namespace":              namespace,
		"experimentName":         experiment.Name,
		"internalComponentImage": r.Configuration.InternalComponentImage,
		"imagePullPolicy":        string(r.Configuration.InternalComponentImagePullPolicy),
	}
	buff, err = processTemplate(buff, values)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("cannot process template for file %s", fn))
	}
	err = yaml.Unmarshal(buff, obj)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("cannot unmarshall file %s", fn))
	}
	obj.SetNamespace(namespace)
	obj.SetName(resName)
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
	annotations["component-source"] = fName
	obj.SetAnnotations(annotations)
	obj.SetOwnerReferences([]metav1.OwnerReference{*metav1.NewControllerRef(experiment, yassv1.GroupVersion.WithKind(experimentKind))})
	if modifier != nil {
		modifier(obj)
	}
	err = r.Create(ctx, obj)
	return err
}

func processTemplate(buff []byte, values map[string]any) ([]byte, error) {
	// process goLang template like helm chart
	tmpl, err := template.New("tmp").Parse(string(buff))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	_values := make(map[string]any)
	_values["Values"] = values
	err = tmpl.Execute(&b, _values)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func (r *Reconciler) updateStatusConditionForExperimentObject(recon *reconciliationStatus, exp *yassv1.Experiment, compName, conditionType string, obj client.Object, extra error) {
	if compName == "" {
		return
	}
	var condition *metav1.Condition
	found := false
	for _, c := range exp.Status.Conditions {
		if c.Type == conditionType {
			condition = c
			found = true
			break
		}
	}
	if !found {
		condition = &metav1.Condition{
			Type:   conditionType,
			Status: metav1.ConditionUnknown,
			Reason: "undefined",
		}
	}
	var newStatus metav1.ConditionStatus
	var newReason string
	newMessage := ""
	if extra != nil {
		newStatus = metav1.ConditionFalse
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
			ready = goutils.AllMatch(x.Status.Conditions, func(element v1.PodCondition) bool {
				return element.Status == v1.ConditionTrue
			})
		case *appsv1.StatefulSet:
			ready = x.Status.AvailableReplicas > 0
			newReason = goutils.BoolToStr(ready, fmt.Sprintf("Replicas_%d", x.Status.AvailableReplicas), "notReadyAtLeastOneReplicaIsRequired")
		case *appsv1.Deployment:
			ready = x.Status.AvailableReplicas > 0
			newReason = goutils.BoolToStr(ready, fmt.Sprintf("Replicas_%d", x.Status.AvailableReplicas), "notReadyAtLeastOneReplicaIsRequired")
		case *yassv1.FsNode:
			// A node that reached a terminal phase has finished its mission; going
			// not-ready afterwards (e.g. an intentional Destroy in UC4) is not a
			// component failure. A genuine Errored node is still caught by
			// evaluateExperimentOutcome's phase aggregation.
			ready = x.Status.Ready || x.Status.Phase.IsTerminal()
		default:
			ready = true
		}
		if ready {
			newReason = "ok"
		}
		newStatus = goutils.BoolTo(ready, metav1.ConditionTrue, metav1.ConditionFalse)
	}

	if condition.Status != newStatus || condition.Reason != newReason || condition.Message != newMessage {
		condition.LastTransitionTime = metav1.Now()
		condition.Reason = newReason
		condition.Status = newStatus
		condition.Message = newMessage
		recon.statusUpdated = true
	}
	if !found {
		exp.Status.Conditions = append(exp.Status.Conditions, condition)
		recon.statusUpdated = true
	}
}

func (r *Reconciler) createFsNodeResource(ctx context.Context, namespace string, experiment *yassv1.Experiment, expDef *yassv1.ExperimentDefinition, layoutItem *yassv1.LayoutSatSpec) (*yassv1.FsNode, error) {
	fsNode := &yassv1.FsNode{}
	err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: layoutItem.FsNodeName}, fsNode)
	if err == nil {
		return fsNode, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	var behaviour *yassv1.Behaviour
	for _, sb := range expDef.Spec.Behaviours {
		if sb.FsNodeName == layoutItem.FsNodeName {
			behaviour = &sb
			break
		}
	}
	if behaviour == nil {
		return nil, fmt.Errorf("cannot find fsNode item in experimentDefinition for '%s'", layoutItem.FsNodeName)
	}
	props := make(map[string]string)
	props = goutils.MapMergeOverride(props, experiment.Spec.FsNodeProperties, layoutItem.Properties)

	fsNode = &yassv1.FsNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      layoutItem.FsNodeName,
			Namespace: namespace,
			Labels: map[string]string{
				controller.LabelExperiment: experiment.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(experiment, yassv1.GroupVersion.WithKind(experimentKind)),
			},
		},
		Spec: yassv1.FsNodeSpec{
			NodeType:         layoutItem.NodeType,
			EmbeddedHardware: layoutItem.EmbeddedHardware,
			EmbeddedPosition: layoutItem.EmbeddedPosition,
			EngineContainers: experiment.Spec.EngineContainers,
			EngineVolumes:    experiment.Spec.EngineVolumes,
			Agent:            behaviour.Agent,
			Properties:       props,
			HardwareEvents:   behaviour.HardwareEvents,
		},
	}
	if err := r.Create(ctx, fsNode); err != nil {
		return nil, err
	}
	return fsNode, nil
}

// errExecutorDNSPending is returned by httpExperimentExecutor when the
// experiment-executor Service has not yet propagated to CoreDNS. This
// is a transient startup-race signal — callers should swallow it,
// record a Normal event, and requeue quickly instead of letting
// controller-runtime apply exponential backoff.
var errExecutorDNSPending = errors.New("experiment-executor service not in cluster DNS yet")

func (r *Reconciler) httpExperimentExecutor(ctx context.Context, recon *reconciliationStatus, endpoint string, body []byte, experiment *yassv1.Experiment) error {
	url := fmt.Sprintf("http://experiment-executor.%s.svc.cluster.local:8080/%s", experiment.Namespace, endpoint)
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	if contentType := goutils.BoolToStr(body != nil, "application/json", ""); contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		if strings.Contains(err.Error(), "no such host") {
			return errExecutorDNSPending
		}
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "ExperimentNotStarted", "unable to start experiment - %s", err)
		return err
	}
	defer goutils.CloseQuietly(response.Body)
	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(response.Body)
		r.recorder.Eventf(experiment, v1.EventTypeWarning, "ExperimentNotStarted", "unable to start experiment - %s", string(body))
	} else {
		body, _ := io.ReadAll(response.Body)
		r.recorder.Eventf(experiment, v1.EventTypeNormal, "ExperimentStarted", "experiment started - %s", string(body))
		r.updateExperimentState(recon, experiment, yassv1.ExperimentStateOngoing)
	}
	return nil
}

// restartBaselineAnnotation stores the total container-restart count across all
// of an experiment's pods observed when the run first reached Ongoing. Restarts
// up to this value happened during startup (before the run was driving) and are
// acceptable; any increase afterwards means a component lost its state.
const restartBaselineAnnotation = "experiment-controller/restart-baseline"

// restartBaseline reads the stored baseline; ok is false when none has been
// recorded yet (or it is unparseable).
func restartBaseline(exp *yassv1.Experiment) (int32, bool) {
	v, ok := exp.Annotations[restartBaselineAnnotation]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return int32(n), true
}

const (
	// evictResourcesAtAnnotation stores the RFC3339 wall-clock deadline at which
	// the experiment's compute resources are evicted. It is stamped once, when the
	// experiment first reaches a terminal state with Spec.EvictResourcesAfter set.
	evictResourcesAtAnnotation = "experiment-controller/evict-resources-at"
	// resourcesEvictedAnnotation marks that the eviction has happened, so the
	// reconciler neither recreates the workloads nor evicts again.
	resourcesEvictedAnnotation = "experiment-controller/resources-evicted"
)

// resourcesEvicted reports whether the experiment's compute resources have
// already been evicted (see Spec.EvictResourcesAfter).
func resourcesEvicted(exp *yassv1.Experiment) bool {
	return exp.Annotations[resourcesEvictedAnnotation] == "true"
}

func isTerminalExperimentState(s yassv1.ExperimentState) bool {
	switch s {
	case yassv1.ExperimentStateSuccess, yassv1.ExperimentStateFailure,
		yassv1.ExperimentStateTimedOut, yassv1.ExperimentStateErrored:
		return true
	}
	return false
}

// maybeEvictResources implements Spec.EvictResourcesAfter: once the experiment
// is terminal it schedules (on first observation) and later performs the
// deletion of all CPU/RAM-consuming resources, keeping the Experiment object so
// cluster capacity is freed without deleting the experiment. It returns a
// non-zero requeue delay while waiting for the deadline.
func (r *Reconciler) maybeEvictResources(ctx context.Context, experiment *yassv1.Experiment) (time.Duration, error) {
	if experiment.Spec.EvictResourcesAfter == nil || experiment.Spec.EvictResourcesAfter.Duration <= 0 {
		return 0, nil
	}
	if !isTerminalExperimentState(experiment.Status.ExperimentState) || resourcesEvicted(experiment) {
		return 0, nil
	}

	deadlineStr, ok := experiment.Annotations[evictResourcesAtAnnotation]
	if !ok {
		deadline := time.Now().Add(experiment.Spec.EvictResourcesAfter.Duration)
		if err := r.setExperimentAnnotation(ctx, experiment, evictResourcesAtAnnotation, deadline.UTC().Format(time.RFC3339)); err != nil {
			return 0, err
		}
		r.recorder.Eventf(experiment, v1.EventTypeNormal, "ResourcesEvictionScheduled",
			"experiment terminal (%s); compute resources will be evicted at %s (evictResourcesAfter=%s)",
			experiment.Status.ExperimentState, deadline.UTC().Format(time.RFC3339), experiment.Spec.EvictResourcesAfter.Duration)
		return requeueUntil(deadline), nil
	}

	deadline, err := time.Parse(time.RFC3339, deadlineStr)
	if err != nil {
		// Corrupt annotation — re-stamp from now.
		return 0, r.setExperimentAnnotation(ctx, experiment, evictResourcesAtAnnotation,
			time.Now().Add(experiment.Spec.EvictResourcesAfter.Duration).UTC().Format(time.RFC3339))
	}
	if time.Until(deadline) > 0 {
		return requeueUntil(deadline), nil
	}

	count, err := r.evictExperimentResources(ctx, experiment.Namespace, experiment.Name)
	if err != nil {
		return 10 * time.Second, errors.Wrap(err, "cannot evict experiment resources")
	}
	if err := r.setExperimentAnnotation(ctx, experiment, resourcesEvictedAnnotation, "true"); err != nil {
		return 0, err
	}
	r.recorder.Eventf(experiment, v1.EventTypeNormal, "ResourcesEvicted",
		"evicted %d compute resource(s) (FsNodes, StatefulSets, Deployments); Experiment kept, cluster capacity freed", count)
	return 0, nil
}

// requeueUntil returns how long to wait before re-checking the eviction
// deadline, capped at one minute so a restarted operator stays responsive.
func requeueUntil(deadline time.Time) time.Duration {
	d := time.Until(deadline)
	if d < time.Second {
		d = time.Second
	}
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

// evictExperimentResources deletes every CPU/RAM-consuming object of the
// experiment — FsNodes (their pods cascade) and the StatefulSets/Deployments of
// the executor, messaging and the shared engine/observability components.
// Services, ConfigMaps, ServiceAccounts and the Experiment itself are kept.
func (r *Reconciler) evictExperimentResources(ctx context.Context, namespace, experimentName string) (int, error) {
	sel := client.MatchingLabels{controller.LabelExperiment: experimentName}
	lists := []client.ObjectList{&yassv1.FsNodeList{}, &appsv1.StatefulSetList{}, &appsv1.DeploymentList{}}
	count := 0
	for _, list := range lists {
		if err := r.List(ctx, list, client.InNamespace(namespace), sel); err != nil {
			return count, err
		}
		items, err := meta.ExtractList(list)
		if err != nil {
			return count, err
		}
		for _, item := range items {
			obj, ok := item.(client.Object)
			if !ok {
				continue
			}
			if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// setExperimentAnnotation sets one annotation on the Experiment and persists it.
func (r *Reconciler) setExperimentAnnotation(ctx context.Context, experiment *yassv1.Experiment, key, value string) error {
	if experiment.Annotations == nil {
		experiment.Annotations = map[string]string{}
	}
	if experiment.Annotations[key] == value {
		return nil
	}
	experiment.Annotations[key] = value
	return r.Update(ctx, experiment)
}

// experimentRestartTotal sums container RestartCount across every pod in the
// experiment's namespace and lists the containers that have restarted (for the
// failure event). An experiment owns its namespace, so this covers all
// components: the experiment-executor, every FsNode pod (world-controller,
// agent, engine) and the shared internal-components (messaging, metrics-bridge,
// ...). A restart of any of them loses in-memory state and breaks the run.
func (r *Reconciler) experimentRestartTotal(ctx context.Context, namespace string) (int32, []string, error) {
	podList := &v1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(namespace)); err != nil {
		return 0, nil, err
	}
	var total int32
	var details []string
	for i := range podList.Items {
		pod := &podList.Items[i]
		for j := range pod.Status.ContainerStatuses {
			cs := &pod.Status.ContainerStatuses[j]
			total += cs.RestartCount
			if cs.RestartCount > 0 {
				details = append(details, fmt.Sprintf("%s/%s=%d", pod.Name, cs.Name, cs.RestartCount))
			}
		}
	}
	return total, details, nil
}

// hasUnschedulablePods reports whether any pod of the experiment is currently
// unschedulable (PodScheduled=False, reason Unschedulable) — i.e. the cluster
// lacks the CPU/memory to place it. Used to surface ExperimentStateInsufficientResources.
func (r *Reconciler) hasUnschedulablePods(ctx context.Context, namespace, experimentName string) (bool, string, error) {
	podList := &v1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels(map[string]string{controller.LabelExperiment: experimentName})); err != nil {
		return false, "", err
	}
	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase != v1.PodPending {
			continue
		}
		for j := range pod.Status.Conditions {
			c := &pod.Status.Conditions[j]
			if c.Type == v1.PodScheduled && c.Status == v1.ConditionFalse && c.Reason == v1.PodReasonUnschedulable {
				return true, fmt.Sprintf("%s: %s", pod.Name, c.Message), nil
			}
		}
	}
	return false, "", nil
}

// evaluateExperimentOutcome transitions an Ongoing/TimedOut experiment to a
// terminal state by aggregating the FsNode phases (set by the world-controller
// from each agent's sentinel file, and by the fs-node controller for crashes):
// any Errored ⇒ Errored; else any MissionFail ⇒ Failure; else (all
// MissionCompleted) ⇒ Success. Acts only once EVERY FsNode is terminal.
func (r *Reconciler) evaluateExperimentOutcome(recon *reconciliationStatus, ctx context.Context, experiment *yassv1.Experiment) error {
	state := experiment.Status.ExperimentState
	if state != yassv1.ExperimentStateOngoing && state != yassv1.ExperimentStateTimedOut {
		return nil
	}
	fsNodes := &yassv1.FsNodeList{}
	if err := r.List(ctx, fsNodes,
		client.InNamespace(experiment.Namespace),
		client.MatchingLabels(map[string]string{controller.LabelExperiment: experiment.Name})); err != nil {
		return err
	}
	if len(fsNodes.Items) == 0 {
		return nil
	}
	anyErrored, anyFailure := false, false
	for i := range fsNodes.Items {
		ph := fsNodes.Items[i].Status.Phase
		if !ph.IsTerminal() {
			return nil // at least one node still running — wait
		}
		switch ph {
		case yassv1.FsNodePhaseErrored:
			anyErrored = true
		case yassv1.FsNodePhaseMissionFail:
			anyFailure = true
		}
	}
	switch {
	case anyErrored:
		r.updateExperimentState(recon, experiment, yassv1.ExperimentStateErrored)
	case anyFailure:
		r.updateExperimentState(recon, experiment, yassv1.ExperimentStateFailure)
	default:
		r.updateExperimentState(recon, experiment, yassv1.ExperimentStateSuccess)
	}
	return nil
}

func (r *Reconciler) updateExperimentState(recon *reconciliationStatus, experiment *yassv1.Experiment, newState yassv1.ExperimentState) {
	oldState := experiment.Status.ExperimentState
	if oldState != newState {
		if oldState == "" {
			oldState = "NONE"
		}
		r.recorder.Eventf(experiment, v1.EventTypeNormal, string(newState), fmt.Sprintf("State transition %s -> %s", oldState, newState))
		experiment.Status.ExperimentState = newState
		recon.statusUpdated = true
	}
}

func kebabToCamel(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return strings.Join(parts, "")
}
