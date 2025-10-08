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
// +kubebuilder:printcolumn:name="DiskSpace",type=string,JSONPath=`.spec.DiskSpace`

// A definition of the hardware installed on the satelite or ground station.
type HardwareDefinition struct {
	metav1.TypeMeta `json:",inline"`

	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Optional
	// Description of the hardware specification.
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Optional
	Spec *HardwareSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type HardwareDefinitionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareDefinition `json:"items"`
}

type HardwareSpec struct {
	// Number of available CPUs.
	CPU *resource.Quantity `json:"cpu"`

	// Number of available RAM.
	Memory *resource.Quantity `json:"memory"`

	// Available disk space. If not defined then unlimited.
	// +kubebuilder:validation:Optional
	DiskSpace *resource.Quantity `json:"diskSpace"`
}
