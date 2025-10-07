package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&Layout{}, &LayoutList{})
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
type Layout struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Description       string          `json:"description,omitempty"`
	Spec              []LayoutSatSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type LayoutList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Layout `json:"items"`
}

// +kubebuilder:validation:XValidation:rule="(has(self.hardwareSpecRef) && !has(self.hardwareSpec)) || (!has(self.hardwareSpecRef) && has(self.hardwareSpec))",message="Exactly one of spec.hardwareSpecRef or spec.hardwareSpec must be set"
// +kubebuilder:validation:XValidation:rule="(has(self.orbit) && !has(self.earthPosition)) || (!has(self.orbit) && has(self.earthPosition))",message="Exactly one of spec.orbit or spec.earthPosition must be set"
type LayoutSatSpec struct {
	SatName string `json:"satName"`

	// +kubebuilder:validation:Optional
	HardwareSpec HardwareSpec `json:"hardwareSpec,omitempty"`

	// +kubebuilder:validation:Optional
	HardwareSpecRef string `json:"hardwareSpecRef,omitempty"`

	EmbeddedPosition EmbeddedPosition `json:",inline"`
}
