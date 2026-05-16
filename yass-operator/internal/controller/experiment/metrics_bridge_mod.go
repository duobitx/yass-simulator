package experiment

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"github.com/duobitx/yass-simulator/yass-operator/internal/controller"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	labelEngine = "yass-engine"
	labelRunID  = "yass-run-id"
)

// modMetricsBridge stamps experiment context (name/engine/run-id) as pod
// labels and env vars on the metrics-bridge Deployment. The bridge reads
// its own runtime knobs (deadline, etc.) from its container defaults.
func modMetricsBridge(experiment *yassv1.Experiment) func(client.Object) {
	engine := deriveEngine(experiment)
	runID := deriveRunID(experiment)

	return func(object client.Object) {
		dep, ok := object.(*appsv1.Deployment)
		if !ok {
			slog.Default().Error(fmt.Sprintf("modMetricsBridge: unsupported type %T", object))
			return
		}
		dep.Spec.Template.Annotations = addExperimentLabel(dep.Spec.Template.Annotations, experiment.Name)

		if dep.Spec.Template.Labels == nil {
			dep.Spec.Template.Labels = map[string]string{}
		}
		dep.Spec.Template.Labels[controller.LabelExperiment] = experiment.Name
		dep.Spec.Template.Labels[labelEngine] = engine
		dep.Spec.Template.Labels[labelRunID] = runID

		envs := []v1.EnvVar{
			{Name: "EXPERIMENT_NAME", Value: experiment.Name},
			{Name: "ENGINE", Value: engine},
			{Name: "RUN_ID", Value: runID},
		}
		for i := range dep.Spec.Template.Spec.Containers {
			c := &dep.Spec.Template.Spec.Containers[i]
			if c.Name != "metrics-bridge" {
				continue
			}
			c.Env = mergeEnvs(c.Env, envs)
		}
	}
}

func deriveEngine(experiment *yassv1.Experiment) string {
	for _, c := range experiment.Spec.EngineContainers {
		name := strings.ToLower(c.Name + " " + c.Image)
		switch {
		case strings.Contains(name, "edfs"):
			return "edfs"
		case strings.Contains(name, "tus"):
			return "tus"
		}
	}
	return "unknown"
}

// deriveRunID returns "<experiment>_<yyyyMMddTHHmmssZ>". The separator and
// stamp use only characters valid in a Kubernetes label value
// ([A-Za-z0-9._-]).
func deriveRunID(experiment *yassv1.Experiment) string {
	t := experiment.CreationTimestamp.UTC()
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return experiment.Name + "_" + t.Format("20060102T150405Z")
}

func mergeEnvs(existing, extra []v1.EnvVar) []v1.EnvVar {
	known := make(map[string]int, len(existing))
	for i, e := range existing {
		known[e.Name] = i
	}
	for _, e := range extra {
		if idx, ok := known[e.Name]; ok {
			existing[idx] = e
			continue
		}
		known[e.Name] = len(existing)
		existing = append(existing, e)
	}
	return existing
}
