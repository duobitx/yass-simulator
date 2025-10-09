package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Experiment{}, &ExperimentList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.spec.ready`
// +kubebuilder:printcolumn:name="Start",type=boolean,JSONPath=`.spec.start`
// +kubebuilder:printcolumn:name="Started",type=date,JSONPath=`.status.started`
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
	// Engine defines what engine will be tested during the experiment.
	Engine SimpleContainer `json:"engine,omitempty"`
	// SimulationStartTime is a starting point of the experiment.
	SimulationStartTime metav1.Time `json:"simulationStartTime"`
	// Start if to start the experiment as soon as it's ready.
	Start bool `json:"start"`
}

// ExperimentStatus defines the desired state of an Experiment
type ExperimentStatus struct {
	// +kubebuilder:validation:Optional
	// Started when it was started (real time)
	Started *metav1.Time `json:"Started"`
	// Ready - ready to be started or already started.
	Ready bool `json:"ready"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
