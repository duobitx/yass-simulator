package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Layout{}, &LayoutList{})
}

// Layout is the *world map* of an experiment: the set of satellites and ground
// stations that exist, where they are physically located, and what hardware they
// run on. A Layout is intentionally behaviour-free — it does not say what the
// nodes should *do*; that is the job of an [ExperimentDefinition].
//
// The same Layout can therefore be paired with several ExperimentDefinitions
// (different agent payloads over the same constellation) to run apples-to-apples
// comparisons — this is the pattern used by `spain-shot/tus` vs `spain-shot/edfs`.
//
// Lifecycle:
//   - Layout is cluster-scoped and authored once by the operator/researcher.
//   - It is consumed when an [Experiment] references it via `spec.layoutDefRef`.
//   - The experiment-executor expands each entry in `spec` into a namespaced
//     [FsNode] (which is in turn reconciled into a Pod with the agent +
//     engine + world-controller containers).
//
// Example excerpt:
//
//	apiVersion: int.esa.yass/v1
//	kind: Layout
//	metadata:
//	  name: spain-shot-layout
//	spec:
//	  - fsNode: oneweb-0008
//	    nodeType: satellite
//	    orbit:
//	      tle: ["1 44059U ...", "2 44059  ..."]
//	    hardwareSpecRef: sentinel-2
//	  - fsNode: estrack-kiruna
//	    nodeType: groundStation
//	    earthPosition: { lat: 67.857, lng: 20.964 }
//	    hardwareSpecRef: ground-station-hwdef
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Start",type=boolean,JSONPath=`.spec.start`
type Layout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Description is a free-form human-readable description of the constellation
	// the Layout models (mission, intent, source of the TLEs, ...). Optional.
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Spec is the list of nodes the world consists of. Keyed by `fsNode` (the
	// node's logical name); duplicate names within a single Layout are rejected
	// by the API server via the listMapKey.
	// +listType=map
	// +listMapKey=fsNode
	Spec []LayoutSatSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type LayoutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Layout `json:"items"`
}

// LayoutSatSpec is a single entry in a [Layout] — one satellite or ground station.
// Despite the historical "Sat" in the type name, it represents *any* FsNode
// (the `nodeType` field distinguishes them).
//
// At minimum each entry must specify:
//   - a unique `fsNode` name (becomes the [FsNode] metadata.name),
//   - a `nodeType` (satellite or groundStation),
//   - a position (orbit for satellites, earthPosition for ground stations — see
//     [EmbeddedPosition]),
//   - a hardware profile (via `hardwareSpecRef` to a [HardwareDefinition] or
//     inline — see [EmbeddedHardware]).
type LayoutSatSpec struct {
	// FsNodeName is the logical name of the node. Must be unique within the
	// Layout and is reused as the resulting FsNode's `metadata.name`, so it must
	// be a valid DNS-1123 label (lowercase alphanumerics and `-`).
	FsNodeName string `json:"fsNode"`

	// NodeType is either `satellite` or `groundStation`. Choosing `satellite`
	// requires `orbit` to be set on the embedded position; `groundStation`
	// requires `earthPosition`.
	NodeType FsNodeType `json:"nodeType"`

	// Properties is an arbitrary key/value map injected as environment variables
	// into both the agent and engine containers of the resulting Pod. Use it for
	// per-node tunables that are not part of the hardware profile (e.g.
	// `IS_TARGET_GS=true`). Merged with — and overridden by — the equivalent map
	// on the Experiment (`spec.fsNodeProperties`).
	// +kubebuilder:validation:Optional
	Properties map[string]string `json:"properties,omitempty"`

	EmbeddedHardware `json:",inline"`

	EmbeddedPosition `json:",inline"`
}
