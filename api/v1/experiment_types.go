package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
//+kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Endpoint",type=string,JSONPath=`.status.endpoint`

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
	Items           []Experiment `json:"items"`
}

// ExperimentSpec defines the desired state of an Experiment
type ExperimentSpec struct {
	// ExperimentDefRef is a reference to an ExperimentDef object
	ExperimentDefRef ExperimentDefReference `json:"experiment_def"`
}

// ExperimentDefReference strictly references an ExperimentDef in this API group
type ExperimentDefReference struct {
	// APIGroup is always org.esa.yass for ExperimentDef
	// +kubebuilder:validation:Enum=org.esa.yass
	// +kubebuilder:default=org.esa.yass
	APIGroup string `json:"apiGroup,omitempty"`

	// Kind is always ExperimentDef
	// +kubebuilder:validation:Enum=ExperimentDef
	// +kubebuilder:default=ExperimentDef
	Kind string `json:"kind,omitempty"`

	// Name of the ExperimentDef resource
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace of the ExperimentDef resource. Defaults to the Experiment's namespace if empty.
	// This is optional to allow same-namespace references.
	Namespace string `json:"namespace,omitempty"`
}

// ExperimentStatus defines the desired state of an Experiment
type ExperimentStatus struct {
	Duration      time.Duration  `json:"experiment_duration"`
	NodePositions []NodePosition `json:"node_positions"`
	Progress      float32        `json:"progress"`
}

type NodePosition struct {
	Name string `json:"name"`
	Pos  string `json:"pos_str"`
}
