package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&ExperimentDefinition{}, &ExperimentDefinitionList{})
}

// ExperimentDefinition is the *scenario* of an experiment: per-node behaviour
// (which agent image runs on which FsNode, with which environment) and any
// scheduled hardware events. It deliberately knows nothing about the physical
// placement of nodes — that is the job of a [Layout] — so the same definition
// can be replayed against different constellations.
//
// One ExperimentDefinition pairs with one Layout inside an [Experiment]. The
// node names in `spec.behaviours[].fsNode` must match `fsNode` entries in the
// chosen Layout; the experiment-executor refuses to start otherwise.
//
// Example:
//
//	apiVersion: int.esa.yass/v1
//	kind: ExperimentDefinition
//	metadata:
//	  name: spain-shot
//	spec:
//	  maxDuration: 6h
//	  behaviours:
//	    - fsNode: oneweb-0008
//	      agent:
//	        image: ghcr.io/duobitx/yass-agent-periodic
//	        envsMap:
//	          PHOTO_TARGETS: "spain:40.4:-3.7:500"
//	          FILE_SIZE: "2G"
//	          MAX_PHOTOS: "1"
//	    - fsNode: estrack-kiruna
//	      agent:
//	        image: ghcr.io/duobitx/yass-agent-receive-only
//	        envsMap:
//	          END_ON_ANY: "true"
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="MaxDuration",type=string,JSONPath=`.spec.maxDuration`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type ExperimentDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Description is a free-form human-readable summary of the scenario.
	Description string                   `json:"description,omitempty"`
	// Spec is the actual scenario body. See [ExperimentDefinitionSpec].
	Spec        ExperimentDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type ExperimentDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExperimentDefinition `json:"items"`
}

// ExperimentDefinitionSpec is the body of an [ExperimentDefinition]. It does
// **not** carry observability / metrics knobs — those live on the
// `metrics-bridge` deployment configuration, not on the CRD (see
// `yass-docs/observability-spec.md`).
type ExperimentDefinitionSpec struct {

	// MaxDuration is the wall-clock budget for the experiment, expressed in
	// Go-duration format (e.g. `6h`, `30m`, `90s`). When exceeded the
	// experiment-executor moves the parent [Experiment] to `TimedOut`. Optional —
	// omit for an open-ended run (then end-of-experiment depends on agent signals
	// such as `END_ON_ANY`).
	// +kubebuilder:validation:Optional
	MaxDuration string `json:"maxDuration,omitempty"`

	// Behaviours describes one agent payload per FsNode. The list is keyed by
	// `fsNode`, so each node may appear at most once. A behaviour MUST exist for
	// every FsNode in the paired Layout; a missing entry means "no agent" which
	// is almost always a misconfiguration.
	// +listType=map
	// +listMapKey=fsNode
	Behaviours []Behaviour `json:"behaviours,omitempty"`
}

// Behaviour binds an agent payload to a single named FsNode. The FsNode name is
// the join key against the [Layout] used in the same [Experiment].
type Behaviour struct {
	// FsNodeName is the name of the satellite or ground station this behaviour
	// targets. Must match a `fsNode` entry in the Layout.
	FsNodeName string `json:"fsNode"`

	// Agent is the user-supplied workload container for this node. See
	// [SimpleContainer]; the agent image is expected to follow the YASS agent
	// conventions (consumes well-known env vars, talks to the world-controller
	// over MQTT, drops files into `/mnt/transfer`).
	Agent SimpleContainer `json:"agent"`

	// HardwareEvents is the list of scheduled hardware faults injected into
	// this node by the world-controller. See [HardwareEvent] and
	// yass-docs/hardware-events-spec.md.
	// +kubebuilder:validation:Optional
	HardwareEvents []HardwareEvent `json:"hardwareEvents,omitempty"`
}
