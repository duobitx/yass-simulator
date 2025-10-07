package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&ExperimentDefinition{}, &ExperimentDefinitionList{})
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Model",type=string,JSONPath=`.spec.model`
// +kubebuilder:printcolumn:name="MaxDuration",type=string,JSONPath=`.spec.maxDuration`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ExperimentDefinition defines behaviour and events for an experiment.
type ExperimentDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Description of the experiment
	Description string                   `json:"description,omitempty"`
	Spec        ExperimentDefinitionSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type ExperimentDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ExperimentDefinition `json:"items"`
}

// ExperimentDefinitionSpec defines the desired state of an ExperimentDefinition
type ExperimentDefinitionSpec struct {
	// +kubebuilder:validation:Optional
	// MaxDuration of the experiment
	MaxDuration    *time.Duration  `json:"maxDuration,omitempty"`
	SatBehaviours  []SatBehaviour  `json:"satBehaviours,omitempty"`
	HardwareEvents []HardwareEvent `json:"HardwareEvents,omitempty"`
}

type SatBehaviour struct {
	// Name of the satellite to be configured.
	SatName string `json:"satName"`
	// Agent on the satellite
	Agent SimpleSatContainerDef `json:"agent"`

	// +kubebuilder:validation:Optional
	// What hardware events to expect during the experiment.
	HardwareEvents []HardwareEvent `json:"hardwareEvents"`
}
type HardwareEvent struct {
	// TODO
}
