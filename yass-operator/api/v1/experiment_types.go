package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Experiment{}, &ExperimentList{})
}

// ResourcesEvictedAnnotation, when set to "true", marks that the experiment's
// compute resources (the experiment-executor, the messaging/mosquitto broker,
// FsNode pods and the shared engine/observability workloads) have been evicted
// — see [ExperimentSpec.EvictResourcesAfter]. The reconciler then neither
// recreates nor re-evicts them, and the experiment no longer occupies the
// namespace's singleton workloads.
const ResourcesEvictedAnnotation = "experiment-controller/resources-evicted"

// ResourcesEvicted reports whether this experiment's compute resources have
// already been evicted (see [ExperimentSpec.EvictResourcesAfter]).
func (e *Experiment) ResourcesEvicted() bool {
	return e.Annotations[ResourcesEvictedAnnotation] == "true"
}

// ExperimentState is the top-level lifecycle phase reported by the
// experiment-executor on [ExperimentStatus.ExperimentState]. The state machine is
// strictly forward-moving:
//
//	Init  --> Ready  --> Ongoing  --> Success | Failure | TimedOut
//	  |<-> InsufficientResources (no cluster capacity; recovers to Init when freed)
//	  \-> Errored (from any state on irrecoverable controller error)
type ExperimentState string

const (
	// ExperimentStateInit — resources are being materialised (FsNodes,
	// experiment-executor StatefulSet, services). No simulated time has elapsed.
	ExperimentStateInit = "Init"
	// ExperimentStateReady — all FsNodes are Ready and the experiment-executor is
	// up; waiting either for `spec.start = true` or for an explicit POST /start.
	ExperimentStateReady = "Ready"
	// ExperimentStateInsufficientResources — one or more pods cannot be scheduled
	// because the cluster lacks capacity (CPU/memory). No simulated time has
	// elapsed. NON-terminal and recoverable: when capacity appears the experiment
	// returns to Init and proceeds (Ready -> Ongoing). Lets an operator (or the
	// sweep driver) see "this experiment is too big for this cluster right now"
	// instead of a silent, never-ending Init.
	ExperimentStateInsufficientResources = "InsufficientResources"
	// ExperimentStateErrored — the operator could not bring the experiment up
	// (missing Layout, mismatched node names, ...). Terminal.
	ExperimentStateErrored = "Errored"
	// ExperimentStateOngoing — the experiment is running. Simulated time is
	// advancing; agents are doing their work.
	ExperimentStateOngoing = "Ongoing"
	// ExperimentStateTimedOut — `maxDuration` elapsed before the experiment
	// reached a completion signal. Terminal.
	ExperimentStateTimedOut = "TimedOut"
	// ExperimentStateSuccess — every agent reported its completion criterion met.
	// Terminal.
	ExperimentStateSuccess = "Success"
	// ExperimentStateFailure — an agent reported a failure (e.g. the receiver
	// gave up before receiving the file). Terminal.
	ExperimentStateFailure = "Failure"
)

// Experiment is the top-level, namespaced resource that runs a single simulated
// scenario. It binds together:
//
//   - a cluster-scoped [Layout] (the world map — which nodes exist and where),
//   - a cluster-scoped [ExperimentDefinition] (the scenario — per-node agent
//     behaviour and timing constraints),
//   - a per-experiment engine container set (the file system under test — TUS,
//     EDFS, ...).
//
// On creation the experiment-controller materialises:
//   - one namespaced [FsNode] per Layout entry, joined with the matching
//     Behaviour from the ExperimentDefinition,
//   - an `experiment-executor` StatefulSet + Service in the same namespace,
//     which orchestrates start/stop, watches agent signals and drives the
//     status transitions described in [ExperimentState].
//
// Side-by-side comparisons (TUS vs EDFS) are usually expressed as two Experiment
// objects in two namespaces sharing the same Layout and ExperimentDefinition.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="experimentTime",type=string,JSONPath=`.status.experimentTime`
// +kubebuilder:printcolumn:name="state",type=string,JSONPath=`.status.experimentState`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="apiServerURL",type=string,JSONPath=`.status.apiServerURL`,priority=1
type Experiment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ExperimentSpec   `json:"spec,omitempty"`
	Status            ExperimentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ExperimentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Description is a free-form description of the list (unused by the operator).
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	Items []Experiment `json:"items"`
}

// ExperimentSpec is the desired state of an [Experiment].
type ExperimentSpec struct {
	// ExperimentDefRef is the name of the cluster-scoped [ExperimentDefinition]
	// that supplies per-node agent behaviour. Required.
	ExperimentDefRef string `json:"experimentDefRef"`

	// LayoutDefRef is the name of the cluster-scoped [Layout] that supplies the
	// constellation topology. Required.
	LayoutDefRef string `json:"layoutDefRef"`

	// SimulationStartTime sets the "epoch" of the simulated clock — the point in
	// real calendar time at which the SGP4 propagator should evaluate orbits when
	// the experiment begins. Optional; defaults to "now". Setting it deterministically
	// is useful for reproducing a run or for aligning a TLE epoch with the
	// experiment's "t=0".
	// +optional
	SimulationStartTime *metav1.Time `json:"simulationStartTime,omitempty"`

	// Start, when true, causes the experiment-executor to begin the run as soon
	// as all FsNodes report Ready. When false the experiment will sit in `Ready`
	// indefinitely until an external HTTP POST /start is issued — handy for
	// manual experiments where the operator wants to attach observers first.
	Start bool `json:"start"`

	// EvictResourcesAfter, when set, is the duration to wait AFTER the experiment
	// reaches a terminal state (Success/Failure/TimedOut/Errored) before the
	// operator deletes every CPU/RAM-consuming resource of the experiment — the
	// FsNode pods, the experiment-executor and the shared engine/observability
	// workloads. The Experiment object itself is kept, so cluster capacity is
	// freed WITHOUT having to delete the experiment. The eviction is recorded as
	// a `ResourcesEvicted` event on the Experiment.
	//
	// To avoid losing data, values below 10m are NOT recommended: the final
	// metrics and events still need time to be scraped and exported before the
	// pods disappear. The documented hard minimum for data collection is 5m;
	// 10m leaves a safety margin.
	//
	// Unset (default) means resources are never auto-evicted.
	// +optional
	EvictResourcesAfter *metav1.Duration `json:"evictResourcesAfter,omitempty"`

	// FsNodeProperties is a key/value map merged into the environment of every
	// FsNode in this experiment (both agent and engine containers). It overrides
	// per-node `properties` set on the Layout entry, and is in turn overridden by
	// values set inside the agent's own [SimpleContainer.Envs].
	// Typical use: shared engine knobs such as `GROUND_STATIONS=estrack-kiruna`.
	// +optional
	FsNodeProperties map[string]string `json:"fsNodeProperties,omitempty"`

	// EngineContainers is the file-system engine under test (TUS, EDFS, ...).
	// At least one container is required. These are propagated to every FsNode's
	// `spec.engineContainers` unless overridden per-node.
	// +kubebuilder:validation:MinItems=1
	EngineContainers []corev1.Container `json:"engineContainers,omitempty"`

	// EngineVolumes are extra `corev1.Volume` entries to attach alongside the
	// engine containers. Propagated to every FsNode.
	// +kubebuilder:validation:Optional
	EngineVolumes []corev1.Volume `json:"engineVolumes,omitempty"`

	// RunID, when set, overrides the auto-generated run identifier
	// `<experimentName>@<creationTimestamp>` that the operator stamps on
	// every metric and event. Authors can use it to bake engine params
	// into the identifier (e.g. `forever-edfs-K3N5-par-on`) so dashboards
	// can multi-select on `run_id` to compare parameter sweeps.
	// See yass-docs/observability-v2-spec.md §G2.
	// +kubebuilder:validation:Optional
	RunID string `json:"runId,omitempty"`
}

// ExperimentStatus is the observed state reported by the experiment-controller.
type ExperimentStatus struct {
	// ExperimentState is the current lifecycle phase. See [ExperimentState] for
	// the state machine.
	ExperimentState ExperimentState `json:"experimentState"`

	// ExperimentTime is the time at which the experiment last transitioned state
	// (typically the start time once `Ongoing`). Useful for computing wall-clock
	// duration of a completed run.
	// +optional
	ExperimentTime metav1.Time `json:"experimentTime,omitempty"`

	// ApiServerURL is the kube-apiserver-relative base path of this experiment's
	// aggregated runtime API (group runtime.esa.yass, served by a kubernetes
	// APIService). Read it with `kubectl get --raw <apiServerURL>`; the runtime
	// subfunctions hang off it as subresources: <url>/time, <url>/events,
	// <url>/start, <url>/fsnodes, <url>/results.
	// +optional
	ApiServerURL string `json:"apiServerURL,omitempty"`

	// Conditions captures the standard set of Kubernetes conditions
	// (`Ready`, `Started`, `Completed`, ...).
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []*metav1.Condition `json:"conditions,omitempty"`
}
