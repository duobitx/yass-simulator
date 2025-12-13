package namespace

import (
	"context"
	"fmt"

	"github.com/duobitx/yass-operator/internal/config"
	"github.com/m-szalik/goutils"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	finalizerName              = "yass/namespace-controller"
	requestedLabel             = "yass-namespace"
	dockerSecretName           = "docker-secret"
	saName                     = "yass-experiment-sa"
	yassClusterRoleBindingName = "yass-experiment-rolebinding"
)

// Yass Namespace reconciles an Namespace object
type NamespaceReconciler struct {
	client.Client
	Configuration   *config.Configuration
	recorder        record.EventRecorder
	Scheme          *runtime.Scheme
	SourceNamespace string
}

// + kubebuilder:,resources=namespaces,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile

func (r *NamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	nsName := req.NamespacedName.Name
	var ns v1.Namespace
	err := r.Get(ctx, req.NamespacedName, &ns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil // ignore not found
		}
		return ctrl.Result{}, err
	}
	if ns.DeletionTimestamp != nil {
		if controllerutil.ContainsFinalizer(&ns, finalizerName) {
			logger.Info("Starting orchestration cleanup of the namespace", "namespace", nsName)
			err = r.removeServiceAccountFromClusterRoleBinding(ctx, nsName)
			if err != nil {
				return ctrl.Result{}, err
			}
			if controllerutil.RemoveFinalizer(&ns, finalizerName) {
				err = r.Update(ctx, &ns)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
		logger.Info("Orchestration cleanup of the namespace completed", "namespace", nsName)
		return ctrl.Result{}, nil
	}

	if !namespaceMatchesCriteria(&ns) {
		return ctrl.Result{}, nil
	}

	logger.Info("Starting orchestration of the namespace", "namespace", nsName)
	// copy docker secret
	var dockerSecret v1.Secret
	err = r.Client.Get(ctx, client.ObjectKey{Namespace: r.SourceNamespace, Name: dockerSecretName}, &dockerSecret)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("cannot get secret from %s/%s:: %w", r.SourceNamespace, dockerSecretName, err)
	}
	dockerSecretCopy := dockerSecret.DeepCopy()
	dockerSecretCopy.Namespace = nsName
	dockerSecretCopy.ResourceVersion = ""
	opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, dockerSecretCopy, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("docker secret created", "opResult", opResult)
	// create service account
	sa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: nsName,
		},
		ImagePullSecrets: []v1.LocalObjectReference{{Name: dockerSecretName}},
	}
	opResult, err = controllerutil.CreateOrUpdate(ctx, r.Client, sa, nil)
	if err != nil {
		return ctrl.Result{}, err
	}
	logger.Info("service account created", "opResult", opResult, "sa", saName)
	// update cluster role binding - add new sa
	var crb rbacv1.ClusterRoleBinding
	err = r.Get(ctx, client.ObjectKey{Name: yassClusterRoleBindingName}, &crb)
	if err != nil {
		return ctrl.Result{}, err
	}
	subject := rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      saName,
		Namespace: nsName,
	}
	alreadyBound := goutils.AnyMatch(crb.Subjects, func(sub rbacv1.Subject) bool {
		return sub.Kind == subject.Kind && sub.Name == subject.Name && sub.Namespace == nsName
	})
	if !alreadyBound {
		crb.Subjects = append(crb.Subjects, subject)
		err = r.Client.Update(ctx, &crb)
		if err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("Cluster role binding updated", "subject", subject, "clusterRoleBinding", yassClusterRoleBindingName)
	}
	if controllerutil.AddFinalizer(&ns, finalizerName) {
		err = r.Client.Update(ctx, &ns)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	logger.Info("Orchestration of the namespace completed", "namespace", nsName)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("experiment-controller")
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Namespace{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(obj client.Object) bool {
			// Watch only Namespaces that have label "x" present (any value)
			if obj == nil {
				return false
			}
			return namespaceMatchesCriteria(obj)
		})).
		Named("yass-namespace-controller").
		Complete(r)
}

func (r *NamespaceReconciler) removeServiceAccountFromClusterRoleBinding(ctx context.Context, nsName string) error {
	var crb rbacv1.ClusterRoleBinding
	err := r.Get(ctx, client.ObjectKey{Name: yassClusterRoleBindingName}, &crb)
	if err != nil {
		return err
	}
	var removedSubject *rbacv1.Subject
	crb.Subjects = goutils.Filter(crb.Subjects, func(sub rbacv1.Subject) bool {
		remove := sub.Kind == "ServiceAccount" && sub.Name == saName && sub.Namespace == nsName
		if remove {
			removedSubject = &sub
		}
		return !remove
	})
	err = r.Client.Update(ctx, &crb)
	if err != nil {
		return err
	}

	logf.FromContext(ctx).Info("Cluster role binding updated - subject removed", "subject", removedSubject, "clusterRoleBinding", yassClusterRoleBindingName)
	return nil
}

func namespaceMatchesCriteria(obj client.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	val, ok := labels[requestedLabel]
	if ok {
		b, err := goutils.ParseBool(val)
		if err != nil {
			logger := logf.FromContext(context.Background())
			logger.Error(err, "invalid boolean value", "value", val)
			return false
		}
		return b
	}
	return false
}
