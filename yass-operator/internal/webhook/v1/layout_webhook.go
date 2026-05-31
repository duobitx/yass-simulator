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
	"strings"

	"github.com/m-szalik/goutils"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
)

// SetupLayoutWebhookWithManager registers the webhook for Layout in the manager.
func SetupLayoutWebhookWithManager(mgr ctrl.Manager) error {
	instance := &LayoutWebhook{}
	return ctrl.NewWebhookManagedBy(mgr).For(&yassv1.Layout{}).
		WithDefaulter(instance).
		WithValidator(instance).
		Complete()
}

type LayoutWebhook struct{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Layout.
func (v *LayoutWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return v.validate(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Layout.
func (v *LayoutWebhook) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	return v.validate(ctx, newObj)
}
func (v *LayoutWebhook) validate(_ context.Context, newObj runtime.Object) (admission.Warnings, error) {
	layout, ok := newObj.(*yassv1.Layout)
	if !ok {
		return nil, fmt.Errorf("expected a Layout object for the newObj but got %T", newObj)
	}
	jeh := goutils.NewJoinErrorHelper()
	for elementIndex, node := range layout.Spec {
		if node.Orbit != nil {
			validateTLE(node.Orbit.TLE, elementIndex, jeh)
		}
		if !isValidNodeType(string(node.NodeType)) {
			jeh.Append(fmt.Errorf("element %d: invalid nodeType %s, can be one of %s", elementIndex, node.NodeType, strings.Join(validNodeTypes, ", ")))
		}
	}
	return []string{}, jeh.AsError()
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Layout.
func (v *LayoutWebhook) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *LayoutWebhook) Default(_ context.Context, obj runtime.Object) error {
	fsNode := obj.(*yassv1.Layout)
	for i := range fsNode.Spec {
		if fsNode.Spec[i].NodeType == "" {
			fsNode.Spec[i].NodeType = yassv1.FsNodeTypeSatellite
		}
	}
	return nil
}
