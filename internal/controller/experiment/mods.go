package experiment

import (
	"fmt"
	"log/slog"

	"github.com/ESA-PhiLab/yass-operator/internal/controller"
	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func _addAnnotation(existing map[string]string, key, val string) map[string]string {
	if existing == nil {
		return map[string]string{key: val}
	}
	existing[key] = val
	return existing
}

func modAddExperimentAnnotation(experimentName string) func(object client.Object) {
	return func(object client.Object) {
		switch obj := object.(type) {
		case *appsv1.Deployment:
			obj.Spec.Template.Annotations = _addAnnotation(obj.Spec.Template.Annotations, controller.LabelExperiment, experimentName)
		case *appsv1.DaemonSet:
			obj.Spec.Template.Annotations = _addAnnotation(obj.Spec.Template.Annotations, controller.LabelExperiment, experimentName)
		case *appsv1.StatefulSet:
			obj.Spec.Template.Annotations = _addAnnotation(obj.Spec.Template.Annotations, controller.LabelExperiment, experimentName)
		default:
			slog.Default().Error(fmt.Sprintf("modAddExperimentAnnotation:: unsupported type %T", obj))
		}
	}
}
