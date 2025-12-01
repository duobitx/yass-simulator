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

package v1

import (
	"context"
	"fmt"

	"github.com/duobitx/yass-operator/internal/validation"
	"github.com/m-szalik/goutils"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	yassv1 "github.com/duobitx/yass-operator/api/v1"
)

// SetupFsNodeWebhookWithManager registers the webhook for FsNode in the manager.
func SetupFsNodeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&yassv1.FsNode{}).
		WithValidator(&FsNodeCustomValidator{}).
		Complete()
}

// FsNodeCustomValidator struct is responsible for validating the FsNode resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type FsNodeCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type FsNode.
func (v *FsNodeCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return v.validate(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type FsNode.
func (v *FsNodeCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return v.validate(ctx, newObj)
}

func (v *FsNodeCustomValidator) validate(_ context.Context, newObj runtime.Object) (admission.Warnings, error) {
	fsnode, ok := newObj.(*yassv1.FsNode)
	if !ok {
		return nil, fmt.Errorf("expected a FsNode object for the newObj but got %T", newObj)
	}
	jah := goutils.NewJoinErrorHelper()
	if fsnode.Spec.Orbit != nil {
		validation.ValidateTLE(fsnode.Spec.Orbit.TLE, 0, jah)
	}
	return []string{}, jah.AsError()
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type FsNode.
func (v *FsNodeCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
