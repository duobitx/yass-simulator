package experiment

import (
	"testing"
	"time"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
	"github.com/duobitx/yass-simulator/yass-operator/internal/controller"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newBridgeDep() *appsv1.Deployment {
	return &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: v1.PodTemplateSpec{
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "metrics-bridge"}},
				},
			},
		},
	}
}

func envByName(c v1.Container, name string) string {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

func TestModMetricsBridgeStampsLabelsAndEnv(t *testing.T) {
	created := time.Date(2026, 5, 16, 14, 2, 0, 0, time.UTC)
	exp := &yassv1.Experiment{
		ObjectMeta: metav1.ObjectMeta{Name: "forever-experiment", CreationTimestamp: metav1.NewTime(created)},
		Spec: yassv1.ExperimentSpec{
			EngineContainers: []v1.Container{{Name: "engine-tus", Image: "ghcr.io/duobitx/yass-tus-fs-engine"}},
		},
	}
	dep := newBridgeDep()
	modMetricsBridge(exp)(dep)

	if got := dep.Spec.Template.Labels[controller.LabelExperiment]; got != "forever-experiment" {
		t.Errorf("yass-experiment label=%q, want forever-experiment", got)
	}
	if got := dep.Spec.Template.Labels[labelEngine]; got != "tus" {
		t.Errorf("yass-engine label=%q, want tus", got)
	}
	wantRunID := "forever-experiment_" + created.Format("20060102T150405Z")
	if got := dep.Spec.Template.Labels[labelRunID]; got != wantRunID {
		t.Errorf("yass-run-id label=%q, want %q", got, wantRunID)
	}

	c := dep.Spec.Template.Spec.Containers[0]
	if envByName(c, "EXPERIMENT_NAME") != "forever-experiment" {
		t.Errorf("EXPERIMENT_NAME=%q", envByName(c, "EXPERIMENT_NAME"))
	}
	if envByName(c, "ENGINE") != "tus" {
		t.Errorf("ENGINE=%q", envByName(c, "ENGINE"))
	}
	if envByName(c, "RUN_ID") != wantRunID {
		t.Errorf("RUN_ID=%q", envByName(c, "RUN_ID"))
	}
}

func TestDeriveEngineEDFS(t *testing.T) {
	exp := &yassv1.Experiment{Spec: yassv1.ExperimentSpec{
		EngineContainers: []v1.Container{
			{Name: "edfs-engine", Image: "ghcr.io/duobitx/yass-edfs-engine"},
		},
	}}
	if got := deriveEngine(exp); got != "edfs" {
		t.Errorf("deriveEngine=%q, want edfs", got)
	}
}
