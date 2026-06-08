package fs_node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// deleteCapture records the grace period the finalizer passes to the pod Delete.
type deleteCapture struct {
	called bool
	grace  *int64
}

func reconcilerForPod(pod *v1.Pod, dc *deleteCapture) *FsNodeReconciler {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	b := fake.NewClientBuilder().WithScheme(scheme)
	if pod != nil {
		b = b.WithObjects(pod)
	}
	c := interceptor.NewClient(b.Build(), interceptor.Funcs{
		Delete: func(_ context.Context, _ client.WithWatch, _ client.Object, opts ...client.DeleteOption) error {
			do := &client.DeleteOptions{}
			for _, o := range opts {
				o.ApplyToDelete(do)
			}
			dc.called = true
			dc.grace = do.GracePeriodSeconds
			return nil
		},
	})
	return &FsNodeReconciler{Client: c}
}

func podReq(ns, name string) ctrl.Request {
	return ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}
}

// A healthy (not-yet-deleting) pod is removed gracefully — no grace-0 override.
func TestRemoveFsNode_HealthyPod_GracefulDelete(t *testing.T) {
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "sat-1"}}
	dc := &deleteCapture{}
	r := reconcilerForPod(pod, dc)
	assert.NoError(t, r.removeFsNode(context.Background(), podReq("ns", "sat-1")))
	assert.True(t, dc.called)
	assert.Nil(t, dc.grace, "healthy pod should keep the default grace period")
}

// A pod already marked for deletion (node gone → un-reapable) must be escalated
// to a grace-0 force delete so the finalizer cannot wedge the namespace teardown.
func TestRemoveFsNode_StuckPod_ForceDelete(t *testing.T) {
	dt := metav1.Now()
	pod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace:         "ns",
		Name:              "sat-2",
		DeletionTimestamp: &dt,
		Finalizers:        []string{"yass.test/keep"}, // fake client requires a finalizer alongside a deletionTimestamp
	}}
	dc := &deleteCapture{}
	r := reconcilerForPod(pod, dc)
	assert.NoError(t, r.removeFsNode(context.Background(), podReq("ns", "sat-2")))
	assert.True(t, dc.called)
	if assert.NotNil(t, dc.grace, "stuck pod must be force-deleted") {
		assert.Equal(t, int64(gracefulPODDeletionTime), *dc.grace)
	}
}

// No pod → nothing to delete, finalizer succeeds.
func TestRemoveFsNode_NoPod_NoDelete(t *testing.T) {
	dc := &deleteCapture{}
	r := reconcilerForPod(nil, dc)
	assert.NoError(t, r.removeFsNode(context.Background(), podReq("ns", "gone")))
	assert.False(t, dc.called)
}
