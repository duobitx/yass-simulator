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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FsNodeType is the discriminator between the two kinds of simulated nodes.
// It governs which position kind is required ([Orbit] vs [EarthPosition]) and
// influences how the world-controller models the node's connectivity windows.
type FsNodeType string

const (
	// FsNodeTypeSatellite — a node propagated along a TLE. Requires `orbit` to be set.
	FsNodeTypeSatellite FsNodeType = "satellite"
	// FsNodeTypeGroundStation — a stationary node fixed to an Earth coordinate.
	// Requires `earthPosition` to be set.
	FsNodeTypeGroundStation FsNodeType = "groundStation"
)

const (
	// AnnotationAgentContainers / AnnotationEngineContainers are pod
	// annotations set by the fs-node controller listing (comma-separated) the
	// agent and engine container names. The world-controller reads them to know
	// which siblings to SIGKILL on a Destroy hardware event, instead of
	// assuming the agent container is literally named "agent". (Annotations,
	// not labels: label values cannot contain commas or exceed 63 chars.)
	AnnotationAgentContainers  = "yass-containers/agent"
	AnnotationEngineContainers = "yass-containers/engine"
)

// FsNodeSpec is the desired state of a single simulated node — one satellite or
// ground station. The fs-node controller reconciles it into a Pod composed of
// three logical pieces:
//
//  1. The **agent** ([SimpleContainer]) — the user-supplied workload that
//     simulates the on-board behaviour (taking pictures, receiving files, ...).
//  2. The **engine containers** (`corev1.Container`) — the file-system engine
//     under test (TUS, EDFS, ...). These get raw access to ports/volumes and
//     therefore use the full corev1 schema.
//  3. The **world-controller** sidecar — injected automatically by the operator.
//     It runs the orbital/visibility math, enforces simulated networking and
//     publishes telemetry (battery, disk, position) on MQTT.
type FsNodeSpec struct {
	// NodeType selects whether this is a satellite or a ground station. See [FsNodeType].
	NodeType FsNodeType `json:"nodeType"`

	// Properties is an arbitrary key/value map injected as environment variables
	// into both the agent and engine containers. The operator merges this with
	// `Experiment.spec.fsNodeProperties` (the experiment-level map wins on conflict)
	// and with the agent's own [SimpleContainer.Envs].
	// +kubebuilder:validation:Optional
	Properties map[string]string `json:"properties,omitempty"`

	EmbeddedHardware `json:",inline"`

	EmbeddedPosition `json:",inline"`

	// Agent is the user-supplied workload describing what this node *does* during
	// the experiment (the satellite payload, or the ground-station receiver). See
	// [SimpleContainer] for the supported fields.
	Agent SimpleContainer `json:"agent,omitempty"`

	// EngineContainers is the file-system engine running on this node (TUS, EDFS,
	// ...). At least one container is required. The operator wires the well-known
	// `/tmp` and `/mnt/transfer` volumes and adds the world-controller sidecar
	// around them.
	// +kubebuilder:validation:MinItems=1
	EngineContainers []corev1.Container `json:"engineContainers,omitempty"`

	// EngineVolumes are extra `corev1.Volume` entries to attach to the Pod for the
	// engine containers' use (e.g. ConfigMaps, additional emptyDirs). The hard
	// `emptyDir.sizeLimit` from the hardware profile is applied separately by the
	// controller and does not need to be set here.
	// +kubebuilder:validation:Optional
	EngineVolumes []corev1.Volume `json:"engineVolumes,omitempty"`

	// HardwareEvents is the list of scheduled hardware faults the
	// world-controller should inject into this node during the
	// experiment. Populated by the experiment-controller from the
	// matching Behaviour.hardwareEvents in the ExperimentDefinition.
	// See yass-docs/hardware-events-spec.md.
	// +kubebuilder:validation:Optional
	HardwareEvents []HardwareEvent `json:"hardwareEvents,omitempty"`
}

// FsNodeStatus is reported by the fs-node controller and reflects the live state
// of the underlying Pod and its simulated counterpart. The "Str" fields are
// human-friendly summaries surfaced in `kubectl get fsn` columns; the structured
// telemetry (battery percentage, disk usage per volume, current power mode, ...)
// is published on the MQTT topic `<fsNode>/resources` rather than into the
// resource status, because the resource status would not handle the update
// frequency gracefully.
// FsNodePhase is the lifecycle phase of an FsNode, covering the whole life of
// its Pod (Pending → Creating → Running) through to a terminal outcome derived
// from the agent: CompletedSuccessfully / CompletedFailure (deliberate, expected
// failure) / Errored (unexpected crash). The operator sets the pre-terminal
// phases from the Pod status; the world-controller sets Running and the terminal
// phases (from the agent's sentinel file).
type FsNodePhase string

const (
	FsNodePhasePending              FsNodePhase = "Pending"
	FsNodePhaseCreating             FsNodePhase = "Creating"
	FsNodePhaseRunning              FsNodePhase = "Running"
	FsNodePhaseCompletedSuccessfully FsNodePhase = "CompletedSuccessfully"
	FsNodePhaseCompletedFailure     FsNodePhase = "CompletedFailure"
	FsNodePhaseErrored              FsNodePhase = "Errored"
)

// IsTerminal reports whether the phase is a terminal outcome.
func (p FsNodePhase) IsTerminal() bool {
	return p == FsNodePhaseCompletedSuccessfully || p == FsNodePhaseCompletedFailure || p == FsNodePhaseErrored
}

type FsNodeStatus struct {
	// Phase is the lifecycle phase of the node (Pending|Creating|Running|
	// CompletedSuccessfully|CompletedFailure|Errored). See FsNodePhase.
	// +optional
	Phase FsNodePhase `json:"phase,omitempty"`
	// Conditions captures the standard set of Kubernetes condition entries
	// (`Ready`, `EngineReady`, ...).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []*metav1.Condition `json:"conditions,omitempty"`
	// Ready is true when the Pod is running and the world-controller has finished
	// its initial position computation.
	Ready bool `json:"ready"`
	// PosStr is a short human-readable summary of the node's current position
	// (e.g. `lat=67.86, lng=20.96` for a ground station, or an over-flight summary
	// for a satellite). Refreshed by the world-controller.
	PosStr string `json:"posStr"`
	// EnergyConsumptionStr is a short human-readable summary of the current
	// battery / energy state (state-of-charge, power mode). Refreshed by the
	// world-controller.
	EnergyConsumptionStr string `json:"batteryStr"`
}

// FsNode represents one simulated satellite or ground station. It is the central
// runtime resource of YASS: every other CRD ultimately exists to describe
// *which* FsNodes should be created, *where* they sit and *what* they should do.
//
// In a normal experiment users do **not** author FsNodes directly — the
// experiment-executor creates them from a [Layout] × [ExperimentDefinition] pair
// referenced by an [Experiment]. FsNodes may, however, be applied by hand for
// ad-hoc tests of the operator itself.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=fsn
// +kubebuilder:resource:shortName=fsns
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="NodeType",type=string,JSONPath=`.spec.nodeType`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Bat",type=string,JSONPath=`.status.batteryStr`
// +kubebuilder:printcolumn:name="PosOverEarth",type=string,JSONPath=`.status.posStr`
type FsNode struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of FsNode
	// +required
	Spec FsNodeSpec `json:"spec"`

	// status defines the observed state of FsNode
	// +optional
	Status FsNodeStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true
// FsNodeList contains a list of FsNode
type FsNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FsNode `json:"items"`
}

func (s *SimpleContainer) AsMap() (map[string]string, error) {
	ret := map[string]string{}
	if s.Envs != nil {
		for k, v := range s.Envs {
			ret[k] = v
		}
	}
	return ret, nil
}

func init() {
	SchemeBuilder.Register(&FsNode{}, &FsNodeList{})
}
