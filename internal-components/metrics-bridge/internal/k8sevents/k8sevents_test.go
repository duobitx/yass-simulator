package k8sevents

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		kind, eventType string
		want            string
	}{
		{"lifecycle", "started", corev1.EventTypeNormal},
		{"lifecycle", "Failure", corev1.EventTypeWarning},
		{"lifecycle", "Errored", corev1.EventTypeWarning},
		{"lifecycle", "TimedOut", corev1.EventTypeWarning},
		{"online_state", "online", corev1.EventTypeNormal},
		{"online_state", "offline", corev1.EventTypeWarning},
		{"power", "enter_shadow", corev1.EventTypeNormal},
		{"power", "enter_low_power", corev1.EventTypeWarning},
		{"power", "exit_low_power", corev1.EventTypeNormal},
		{"hardware", "antenna-deploy", corev1.EventTypeNormal},
		{"hardware", "antenna-failure", corev1.EventTypeWarning},
		{"hardware", "sensor_error", corev1.EventTypeWarning},
		{"crud", "PUT", corev1.EventTypeNormal},
		{"crud", "DELETE", corev1.EventTypeNormal},
	}
	for _, c := range cases {
		if got := classify(c.kind, c.eventType); got != c.want {
			t.Errorf("classify(%q,%q) = %q, want %q", c.kind, c.eventType, got, c.want)
		}
	}
}

func TestBuildReason(t *testing.T) {
	cases := map[string]string{
		"crud|PUT":                 "CrudPUT",
		"lifecycle|started":        "LifecycleStarted",
		"power|enter_low_power":    "PowerEnterLowPower",
		"online_state|offline":     "OnlineStateOffline",
		"hardware|antenna-failure": "HardwareAntennaFailure",
	}
	for in, want := range cases {
		kind, evt := "", ""
		for i := 0; i < len(in); i++ {
			if in[i] == '|' {
				kind, evt = in[:i], in[i+1:]
				break
			}
		}
		if got := buildReason(kind, evt); got != want {
			t.Errorf("buildReason(%q,%q) = %q, want %q", kind, evt, got, want)
		}
	}
}

func TestBuildMessage(t *testing.T) {
	cases := []struct {
		kind, eventType string
		payload         map[string]any
		want            string
	}{
		{"crud", "PUT", map[string]any{"fsNode": "sat-1", "name": "photo.jpg", "size": 1024}, "PUT photo.jpg on sat-1 (1024B)"},
		{"crud", "DELETE", map[string]any{"fsNode": "sat-1"}, "DELETE on sat-1"},
		{"online_state", "offline", map[string]any{"fsNode": "sat-1"}, "sat-1 went offline"},
		{"power", "enter_shadow", map[string]any{"fsNode": "sat-1", "batteryWh": 42.5}, "sat-1 enter_shadow (batteryWh=42.5)"},
		{"lifecycle", "started", map[string]any{}, "Experiment state: started"},
		{"lifecycle", "Failure", map[string]any{"reason": "agent-timeout"}, "Experiment state: Failure (agent-timeout)"},
		{"hardware", "antenna-deploy", map[string]any{"fsNode": "gs-a"}, "gs-a: antenna-deploy"},
	}
	for _, c := range cases {
		if got := buildMessage(c.kind, c.eventType, c.payload); got != c.want {
			t.Errorf("buildMessage(%q,%q,%v) = %q, want %q", c.kind, c.eventType, c.payload, got, c.want)
		}
	}
}

func TestNoopEmitterDoesNotPanic(t *testing.T) {
	Noop().Emit("crud", "PUT", nil)
	Noop().Emit("crud", "PUT", map[string]any{"fsNode": "x"})
}
