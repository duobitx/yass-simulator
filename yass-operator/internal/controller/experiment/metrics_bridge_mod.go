package experiment

import (
	"encoding/json"
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

// modMetricsBridge injects experiment context into the metrics-bridge
// Deployment: pod labels for Prometheus relabel rules, env vars for the
// bridge process itself, and the inherited experiment annotation. RunID
// is derived from the Experiment metadata so it is stable per CR.
func modMetricsBridge(experiment *yassv1.Experiment, exDef *yassv1.ExperimentDefinition) func(client.Object) {
	engine := deriveEngine(experiment)
	runID := deriveRunID(experiment)
	targetGSJSON, deliveryDeadline := metricsConfig(exDef)

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
		if targetGSJSON != "" {
			envs = append(envs, v1.EnvVar{Name: "TARGET_GS_BY_FSNODE", Value: targetGSJSON})
		}
		if deliveryDeadline != "" {
			envs = append(envs, v1.EnvVar{Name: "DELIVERY_DEADLINE", Value: deliveryDeadline})
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

func deriveRunID(experiment *yassv1.Experiment) string {
	stamp := experiment.CreationTimestamp.UTC().Format(time.RFC3339)
	if stamp == "" {
		stamp = time.Now().UTC().Format(time.RFC3339)
	}
	return experiment.Name + "@" + stamp
}

func metricsConfig(exDef *yassv1.ExperimentDefinition) (targetGSJSON, deliveryDeadline string) {
	if exDef == nil || exDef.Spec.MetricsConfig == nil {
		return "", ""
	}
	mc := exDef.Spec.MetricsConfig
	if len(mc.TargetGroundStations) > 0 {
		buf, err := json.Marshal(mc.TargetGroundStations)
		if err != nil {
			slog.Default().Warn("modMetricsBridge: cannot marshal targetGroundStations", "error", err)
		} else {
			targetGSJSON = string(buf)
		}
	}
	deliveryDeadline = mc.DeliveryDeadline
	return
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
