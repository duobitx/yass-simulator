package experiment

import (
	"fmt"
	"log/slog"

	"github.com/duobitx/yass-simulator/yass-operator/internal/controller"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func addExperimentLabel(annotations map[string]string, experimentName string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[controller.LabelExperiment] = experimentName
	return annotations
}

func modAddExperimentAnnotation(experimentName string) func(object client.Object) {
	return func(object client.Object) {
		switch obj := object.(type) {
		case *appsv1.Deployment:
			obj.Spec.Template.Annotations = addExperimentLabel(obj.Spec.Template.Annotations, experimentName)
		case *appsv1.DaemonSet:
			obj.Spec.Template.Annotations = addExperimentLabel(obj.Spec.Template.Annotations, experimentName)
		case *appsv1.StatefulSet:
			obj.Spec.Template.Annotations = addExperimentLabel(obj.Spec.Template.Annotations, experimentName)
		case *v1.ConfigMap:
			obj.Annotations = addExperimentLabel(obj.Annotations, experimentName)
		default:
			slog.Default().Error(fmt.Sprintf("modAddExperimentAnnotation:: unsupported type %T", obj))
		}
	}
}
