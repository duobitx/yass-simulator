package controller

import (
	"context"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamespaceTerminating reports whether the named namespace is being deleted.
// The client is the manager's cache-backed reader, so this is cheap to call on
// every reconcile. A namespace under deletion rejects every create with
// "... is forbidden ... because it is being terminated", so a controller that
// keeps trying to (re)create child objects there just error-loops; the
// reconcile should bail out instead. A missing namespace counts as terminating
// — there is nothing left to reconcile into.
func NamespaceTerminating(ctx context.Context, c client.Client, name string) (bool, error) {
	var ns v1.Namespace
	if err := c.Get(ctx, types.NamespacedName{Name: name}, &ns); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return ns.DeletionTimestamp != nil, nil
}
