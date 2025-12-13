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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	yassv1 "github.com/duobitx/yass-operator/api/v1"
)

// log is for logging in this package.
var experimentlog = logf.Log.WithName("experiment-resource")

// SetupExperimentWebhookWithManager registers the webhook for Experiment in the manager.
func SetupExperimentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&yassv1.Experiment{}).
		WithValidator(&ExperimentCustomValidator{}).
		WithDefaulter(&ExperimentCustomDefaulter{}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-int-esa-yass-v1-experiment,mutating=true,failurePolicy=fail,sideEffects=None,groups=int.esa.yass,resources=experiments,verbs=create;update,versions=v1,name=mexperiment-v1.kb.io,admissionReviewVersions=v1

// ExperimentCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Experiment when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ExperimentCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting
}

var _ webhook.CustomDefaulter = &ExperimentCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind Experiment.
func (d *ExperimentCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	experiment, ok := obj.(*yassv1.Experiment)

	if !ok {
		return fmt.Errorf("expected an Experiment object but got %T", obj)
	}
	experimentlog.Info("Defaulting for Experiment", "name", experiment.GetName())

	// TODO(user): fill in your defaulting logic.

	return nil
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-int-esa-yass-v1-experiment,mutating=false,failurePolicy=fail,sideEffects=None,groups=int.esa.yass,resources=experiments,verbs=create;update,versions=v1,name=vexperiment-v1.kb.io,admissionReviewVersions=v1

// ExperimentCustomValidator struct is responsible for validating the Experiment resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ExperimentCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

var _ webhook.CustomValidator = &ExperimentCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type Experiment.
func (v *ExperimentCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	experiment, ok := obj.(*yassv1.Experiment)
	if !ok {
		return nil, fmt.Errorf("expected a Experiment object but got %T", obj)
	}
	experimentlog.Info("Validation for Experiment upon creation", "name", experiment.GetName())
	return v.validateModification(ctx, obj)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type Experiment.
func (v *ExperimentCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	experiment, ok := newObj.(*yassv1.Experiment)
	if !ok {
		return nil, fmt.Errorf("expected a Experiment object for the newObj but got %T", newObj)
	}
	experimentlog.Info("Validation for Experiment upon update", "name", experiment.GetName())
	return v.validateModification(ctx, newObj)
}

func (v *ExperimentCustomValidator) validateModification(_ context.Context, newObj runtime.Object) (admission.Warnings, error) {
	experiment, ok := newObj.(*yassv1.Experiment)
	if !ok {
		return nil, fmt.Errorf("expected a Experiment object for the newObj but got %T", newObj)
	}
	experimentlog.Info("Validation for Experiment upon create/update", "name", experiment.GetName())

	// TODO(user): fill in your validation logic upon object update.
	warnings := admission.Warnings{}
	if experiment.Spec.SimulationStartTime.IsZero() {
		warnings = append(warnings, ".spec.simulationStartTime is empty")
	}
	return warnings, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type Experiment.
func (v *ExperimentCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	experiment, ok := obj.(*yassv1.Experiment)
	if !ok {
		return nil, fmt.Errorf("expected a Experiment object but got %T", obj)
	}
	experimentlog.Info("Validation for Experiment upon deletion", "name", experiment.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
