package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Experiment{}, &ExperimentList{})
}

type ExperimentState string

const (
	ExperimentStateInit      = "Init"
	ExperimentStateReady     = "Ready"
	ExperimentStateErrored   = "Errored"
	ExperimentStateOngoing   = "Ongoing"
	ExperimentStateCompleted = "Completed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="experimentTime",type=string,JSONPath=`.status.experimentTime`
// +kubebuilder:printcolumn:name="state",type=string,JSONPath=`.status.experimentState`
// Experiment main object for simulation.
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

	// +kubebuilder:validation:Optional
	// Description of the experiment
	Description string `json:"description,omitempty"`

	Items []Experiment `json:"items"`
}

// ExperimentSpec defines the desired state of an Experiment
type ExperimentSpec struct {
	// Reference to ExperimentDefinition resource.
	ExperimentDefRef string `json:"experimentDefRef"`
	// Reference to Layout resource.
	LayoutDefRef string `json:"layoutDefRef"`
	// SimulationStartTime is a starting point of the experiment.
	// +optional
	SimulationStartTime *metav1.Time `json:"simulationStartTime,omitempty"`
	// Start if to start the experiment as soon as it's ready.
	Start bool `json:"start"`
	// FsNodeProperties is a map of properties to be set on all fsNodes in this experiment.
	// +optional
	FsNodeProperties map[string]string `json:"fsNodeProperties,omitempty"`

	// What file system engine to be installed
	// +kubebuilder:validation:MinItems=1
	EngineContainers []corev1.Container `json:"engineContainers,omitempty"`
	// +kubebuilder:validation:Optional
	EngineVolumes []corev1.Volume `json:"engineVolumes,omitempty"`
}

// ExperimentStatus defines the desired state of an Experiment
type ExperimentStatus struct {
	ExperimentState ExperimentState `json:"experimentState"`

	ExperimentTime metav1.Time `json:"experimentTime"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []*metav1.Condition `json:"conditions,omitempty"`
}
