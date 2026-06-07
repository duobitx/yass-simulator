package internal

import (
	"context"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// experimentREST is the read-through storage for the base `experiments`
// resource: GET returns the full Experiment CR (decision J). It is get-only —
// the per-experiment apiServerURL always carries a name, and the runtime
// functions live on its subresources.
type experimentREST struct {
	backend *Backend
	table   rest.TableConvertor
}

func newExperimentREST(b *Backend) *experimentREST {
	return &experimentREST{backend: b, table: rest.NewDefaultTableConvertor(groupResource())}
}

var (
	_ rest.Storage              = &experimentREST{}
	_ rest.Scoper               = &experimentREST{}
	_ rest.Getter               = &experimentREST{}
	_ rest.TableConvertor       = &experimentREST{}
	_ rest.KindProvider         = &experimentREST{}
	_ rest.SingularNameProvider = &experimentREST{}
)

func (r *experimentREST) New() runtime.Object     { return &yassv1.Experiment{} }
func (r *experimentREST) Destroy()                {}
func (r *experimentREST) NamespaceScoped() bool   { return true }
func (r *experimentREST) Kind() string            { return kind }
func (r *experimentREST) GetSingularName() string { return singular }

func (r *experimentREST) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	ns, _ := request.NamespaceFrom(ctx)
	exp := &yassv1.Experiment{}
	if err := r.backend.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, exp); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, apierrors.NewNotFound(groupResource(), name)
		}
		return nil, err
	}
	exp.GetObjectKind().SetGroupVersionKind(GroupVersion.WithKind(kind))
	return exp, nil
}

func (r *experimentREST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return r.table.ConvertToTable(ctx, object, tableOptions)
}
