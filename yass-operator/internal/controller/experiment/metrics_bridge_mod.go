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
	labelLayout = "yass-layout"
)

// modMetricsBridge stamps experiment context (name/engine/run-id) as pod
// labels and env vars on the metrics-bridge Deployment. It also injects
// DELIVERY_DEADLINE (when non-empty) so the bridge's PUT->RECEIVED pairing
// window tracks this experiment's maxDuration instead of the container
// default — otherwise slow (e.g. EDFS relay) deliveries are evicted before
// they can be paired and their delivery_seconds is never recorded.
func modMetricsBridge(experiment *yassv1.Experiment, deliveryDeadline string) func(client.Object) {
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
		dep.Spec.Template.Labels[labelLayout] = experiment.Spec.LayoutDefRef

		envs := []v1.EnvVar{
			{Name: "EXPERIMENT_NAME", Value: experiment.Name},
			{Name: "ENGINE", Value: engine},
			{Name: "RUN_ID", Value: runID},
			{Name: "LAYOUT", Value: experiment.Spec.LayoutDefRef},
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

// deliveryDeadlineFor returns the metrics-bridge DELIVERY_DEADLINE for an
// experiment: its maxDuration plus a 10% margin, in Go-duration format.
// A delivery can only occur while the experiment is alive (<= maxDuration),
// so the margin guarantees no in-run delivery is evicted before pairing.
// Returns "" when maxDuration is empty or unparseable, so the bridge keeps
// its own container default.
func deliveryDeadlineFor(maxDuration string) string {
	if maxDuration == "" {
		return ""
	}
	d, err := time.ParseDuration(maxDuration)
	if err != nil || d <= 0 {
		return ""
	}
	return (d + d/10).String()
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

// deriveRunID returns the experiment's user-set RunID when present
// (yass-docs/observability-v2-spec.md §G2), otherwise the auto-stamp
// "<experiment>_<yyyyMMddTHHmmssZ>". Auto-stamp uses only characters
// valid in a Kubernetes label value ([A-Za-z0-9._-]).
func deriveRunID(experiment *yassv1.Experiment) string {
	if experiment.Spec.RunID != "" {
		return experiment.Spec.RunID
	}
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
