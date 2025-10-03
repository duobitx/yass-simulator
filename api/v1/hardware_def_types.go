package v1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	SchemeBuilder.Register(&HardwareDefinition{}, &HardwareDefinitionList{})
}

// +kubebuilder:resource:scope=Cluster
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="CPU",type=string,JSONPath=`.spec.CPU`
// +kubebuilder:printcolumn:name="Memory",type=string,JSONPath=`.spec.Memory`

type HardwareDefinition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Description       string       `json:"description,omitempty"`
	Spec              HardwareSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type HardwareDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareDefinition `json:"items"`
}

type HardwareSpec struct {
	CPU       *resource.Quantity `json:"CPU"`
	Memory    *resource.Quantity `json:"Memory"`
	DiskSpace *resource.Quantity `json:"DiskSpace"`
}
