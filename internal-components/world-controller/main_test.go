package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	yassv1 "github.com/duobitx/yass-simulator/yass-operator/api/v1"
)

// TestAgentVerdictDestroyedNeverErrors pins the Destroy exception: a destroyed
// node reports a terminal NON-errored phase regardless of the agent's exit
// sentinel — even an explicit failure sentinel must be ignored.
func TestAgentVerdictDestroyedNeverErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agent.exit.failure"), []byte("boom"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &appType{}
	a.destroyed.Store(true)
	phase, _, decided := a.agentVerdict(context.Background(), dir)
	if !decided {
		t.Fatal("destroyed node: expected a decided verdict")
	}
	if phase != yassv1.FsNodePhaseMissionCompleted {
		t.Fatalf("destroyed node verdict = %q, want %q (must not be Errored)", phase, yassv1.FsNodePhaseMissionCompleted)
	}
	if phase == yassv1.FsNodePhaseErrored {
		t.Fatal("destroyed node must never be Errored")
	}
}

// TestAgentVerdictSentinels confirms the normal (non-destroyed) sentinel paths
// are unaffected by the Destroy short-circuit.
func TestAgentVerdictSentinels(t *testing.T) {
	cases := []struct {
		sentinel string
		want     yassv1.FsNodePhase
	}{
		{"agent.exit.ok", yassv1.FsNodePhaseMissionCompleted},
		{"agent.exit.failure", yassv1.FsNodePhaseMissionFail},
	}
	for _, c := range cases {
		t.Run(c.sentinel, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, c.sentinel), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			a := &appType{}
			phase, _, decided := a.agentVerdict(context.Background(), dir)
			if !decided || phase != c.want {
				t.Fatalf("%s -> (%q, decided=%v), want %q", c.sentinel, phase, decided, c.want)
			}
		})
	}
}
